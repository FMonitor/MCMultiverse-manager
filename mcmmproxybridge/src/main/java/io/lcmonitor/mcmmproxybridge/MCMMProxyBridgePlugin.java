package io.lcmonitor.mcmmproxybridge;

import com.google.inject.Inject;
import com.velocitypowered.api.event.Subscribe;
import com.velocitypowered.api.event.proxy.ProxyInitializeEvent;
import com.velocitypowered.api.event.proxy.ProxyShutdownEvent;
import com.velocitypowered.api.plugin.Plugin;
import com.velocitypowered.api.plugin.annotation.DataDirectory;
import com.velocitypowered.api.proxy.Player;
import com.velocitypowered.api.proxy.ProxyServer;
import com.velocitypowered.api.proxy.server.RegisteredServer;
import com.velocitypowered.api.proxy.server.ServerInfo;
import org.slf4j.Logger;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.Map;
import java.util.Optional;
import java.util.Properties;
import java.util.StringJoiner;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

@Plugin(
        id = "mcmmproxybridge",
        name = "MCMMProxyBridge",
        version = "0.1.0",
        authors = {"LCMonitor"}
)
public final class MCMMProxyBridgePlugin {
    private final ProxyServer proxyServer;
    private final Logger logger;
    private final Path dataDirectory;
    private HttpServer httpServer;
    private ExecutorService executor;
    private BridgeConfig config;

    @Inject
    public MCMMProxyBridgePlugin(ProxyServer proxyServer, Logger logger, @DataDirectory Path dataDirectory) {
        this.proxyServer = proxyServer;
        this.logger = logger;
        this.dataDirectory = dataDirectory;
    }

    @Subscribe
    public void onProxyInit(ProxyInitializeEvent event) {
        try {
            this.config = loadConfig();
            startHttpServer();
            logger.info("MCMMProxyBridge started on {}:{}", config.listenHost, config.listenPort);
        } catch (Exception e) {
            logger.error("Failed to start MCMMProxyBridge", e);
        }
    }

    @Subscribe
    public void onProxyShutdown(ProxyShutdownEvent event) {
        if (httpServer != null) {
            httpServer.stop(0);
        }
        if (executor != null) {
            executor.shutdownNow();
        }
    }

    private BridgeConfig loadConfig() throws IOException {
        Files.createDirectories(dataDirectory);
        Path path = dataDirectory.resolve("config.properties");
        if (Files.notExists(path)) {
            try (InputStream in = getClass().getResourceAsStream("/config.properties")) {
                if (in == null) {
                    throw new IOException("embedded config.properties not found");
                }
                Files.copy(in, path);
            }
        }
        Properties p = new Properties();
        try (InputStream in = Files.newInputStream(path)) {
            p.load(in);
        }
        BridgeConfig cfg = new BridgeConfig();
        cfg.listenHost = p.getProperty("listen_host", "0.0.0.0").trim();
        cfg.listenPort = Integer.parseInt(p.getProperty("listen_port", "19132").trim());
        cfg.authHeader = p.getProperty("auth_header", "Authorization").trim();
        cfg.authToken = p.getProperty("auth_token", "").trim();
        return cfg;
    }

    private void startHttpServer() throws IOException {
        InetSocketAddress bind = new InetSocketAddress(config.listenHost, config.listenPort);
        this.httpServer = HttpServer.create(bind, 0);
        this.executor = Executors.newFixedThreadPool(4);
        this.httpServer.setExecutor(executor);

        httpServer.createContext("/v1/proxy/register", this::handleRegister);
        httpServer.createContext("/v1/proxy/unregister", this::handleUnregister);
        httpServer.createContext("/v1/proxy/send", this::handleSend);
        httpServer.createContext("/v1/proxy/servers", this::handleList);
        httpServer.createContext("/v1/proxy/players", this::handlePlayers);
        httpServer.start();
    }

    private void handleRegister(HttpExchange ex) throws IOException {
        if (!authorize(ex) || !requirePost(ex)) {
            return;
        }
        Map<String, String> form = parseForm(ex);
        String serverId = form.getOrDefault("server_id", "").trim();
        String host = form.getOrDefault("host", "").trim();
        String portRaw = form.getOrDefault("port", "").trim();
        if (serverId.isEmpty() || host.isEmpty() || portRaw.isEmpty()) {
            writeJson(ex, 400, "{\"status\":\"error\",\"message\":\"missing server_id/host/port\"}");
            return;
        }
        int port;
        try {
            port = Integer.parseInt(portRaw);
        } catch (NumberFormatException nfe) {
            writeJson(ex, 400, "{\"status\":\"error\",\"message\":\"invalid port\"}");
            return;
        }

        Optional<RegisteredServer> existing = proxyServer.getServer(serverId);
        if (existing.isPresent()) {
            proxyServer.unregisterServer(existing.get().getServerInfo());
        }
        ServerInfo info = new ServerInfo(serverId, new InetSocketAddress(host, port));
        proxyServer.registerServer(info);
        writeJson(ex, 200, "{\"status\":\"ok\",\"message\":\"registered\"}");
    }

    private void handleUnregister(HttpExchange ex) throws IOException {
        if (!authorize(ex) || !requirePost(ex)) {
            return;
        }
        Map<String, String> form = parseForm(ex);
        String serverId = form.getOrDefault("server_id", "").trim();
        if (serverId.isEmpty()) {
            writeJson(ex, 400, "{\"status\":\"error\",\"message\":\"missing server_id\"}");
            return;
        }
        Optional<RegisteredServer> existing = proxyServer.getServer(serverId);
        if (existing.isPresent()) {
            proxyServer.unregisterServer(existing.get().getServerInfo());
        }
        writeJson(ex, 200, "{\"status\":\"ok\",\"message\":\"unregistered\"}");
    }

    private void handleSend(HttpExchange ex) throws IOException {
        if (!authorize(ex) || !requirePost(ex)) {
            return;
        }
        Map<String, String> form = parseForm(ex);
        String playerName = form.getOrDefault("player", "").trim();
        String serverId = form.getOrDefault("server_id", "").trim();
        if (playerName.isEmpty() || serverId.isEmpty()) {
            writeJson(ex, 400, "{\"status\":\"error\",\"message\":\"missing player/server_id\"}");
            return;
        }
        Optional<Player> player = proxyServer.getPlayer(playerName);
        if (player.isEmpty()) {
            writeJson(ex, 404, "{\"status\":\"error\",\"message\":\"player not found\"}");
            return;
        }
        Optional<RegisteredServer> target = proxyServer.getServer(serverId);
        if (target.isEmpty()) {
            writeJson(ex, 404, "{\"status\":\"error\",\"message\":\"server not found\"}");
            return;
        }
        player.get().createConnectionRequest(target.get()).fireAndForget();
        writeJson(ex, 200, "{\"status\":\"ok\",\"message\":\"sent\"}");
    }

    private void handleList(HttpExchange ex) throws IOException {
        if (!authorize(ex)) {
            return;
        }
        StringJoiner joiner = new StringJoiner(",", "[", "]");
        for (RegisteredServer s : proxyServer.getAllServers()) {
            ServerInfo info = s.getServerInfo();
            joiner.add(String.format("{\"id\":\"%s\",\"host\":\"%s\",\"port\":%d}",
                    escape(info.getName()),
                    escape(info.getAddress().getHostString()),
                    info.getAddress().getPort()));
        }
        writeJson(ex, 200, "{\"status\":\"ok\",\"servers\":" + joiner + "}");
    }

    private void handlePlayers(HttpExchange ex) throws IOException {
        if (!authorize(ex)) {
            return;
        }
        String query = ex.getRequestURI().getQuery();
        String serverId = "";
        if (query != null && !query.isEmpty()) {
            for (String p : query.split("&")) {
                int idx = p.indexOf('=');
                if (idx > 0 && "server_id".equalsIgnoreCase(p.substring(0, idx))) {
                    serverId = URLDecoder.decode(p.substring(idx + 1), StandardCharsets.UTF_8);
                }
            }
        }
        if (serverId.isEmpty()) {
            writeJson(ex, 400, "{\"status\":\"error\",\"message\":\"missing server_id\"}");
            return;
        }
        Optional<RegisteredServer> target = proxyServer.getServer(serverId);
        if (target.isEmpty()) {
            writeJson(ex, 404, "{\"status\":\"error\",\"message\":\"server not found\"}");
            return;
        }
        StringJoiner players = new StringJoiner(",", "[", "]");
        for (Player p : target.get().getPlayersConnected()) {
            players.add("\"" + escape(p.getUsername()) + "\"");
        }
        writeJson(ex, 200, "{\"status\":\"ok\",\"players\":" + players + "}");
    }

    private boolean authorize(HttpExchange ex) throws IOException {
        if (config.authToken.isEmpty()) {
            return true;
        }
        String value = ex.getRequestHeaders().getFirst(config.authHeader);
        String expected = "Bearer " + config.authToken;
        if (value == null || (!value.equals(config.authToken) && !value.equals(expected))) {
            writeJson(ex, 401, "{\"status\":\"error\",\"message\":\"unauthorized\"}");
            return false;
        }
        return true;
    }

    private boolean requirePost(HttpExchange ex) throws IOException {
        if (!"POST".equalsIgnoreCase(ex.getRequestMethod())) {
            writeJson(ex, 405, "{\"status\":\"error\",\"message\":\"method not allowed\"}");
            return false;
        }
        return true;
    }

    private Map<String, String> parseForm(HttpExchange ex) throws IOException {
        byte[] body = ex.getRequestBody().readAllBytes();
        String raw = new String(body, StandardCharsets.UTF_8);
        Map<String, String> out = new HashMap<>();
        if (raw.isEmpty()) {
            return out;
        }
        String[] parts = raw.split("&");
        for (String part : parts) {
            int idx = part.indexOf('=');
            if (idx < 0) {
                continue;
            }
            String key = URLDecoder.decode(part.substring(0, idx), StandardCharsets.UTF_8);
            String val = URLDecoder.decode(part.substring(idx + 1), StandardCharsets.UTF_8);
            out.put(key, val);
        }
        return out;
    }

    private void writeJson(HttpExchange ex, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        ex.getResponseHeaders().set("Content-Type", "application/json");
        ex.sendResponseHeaders(status, bytes.length);
        try (OutputStream out = ex.getResponseBody()) {
            out.write(bytes);
        }
    }

    private String escape(String s) {
        return s.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static final class BridgeConfig {
        String listenHost;
        int listenPort;
        String authHeader;
        String authToken;
    }
}
