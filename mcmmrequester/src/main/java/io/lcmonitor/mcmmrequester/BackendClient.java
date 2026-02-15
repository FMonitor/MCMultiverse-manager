package io.lcmonitor.mcmmrequester;

import java.io.IOException;
import java.io.OutputStream;
import java.io.UnsupportedEncodingException;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Scanner;
import java.util.UUID;

public final class BackendClient {
    private final String baseUrl;
    private final String token;
    private final int timeoutMs;

    public BackendClient(String baseUrl, String token, int timeoutMs) {
        this.baseUrl = trimTrailingSlash(baseUrl);
        this.token = token == null ? "" : token.trim();
        this.timeoutMs = Math.max(timeoutMs, 1000);
    }

    public BackendResponse postWorldAction(String action, String actorUuid, String actorName, String worldAlias, String targetName)
            throws IOException {
        WorldAction req = new WorldAction(action, actorUuid, actorName);
        req.worldAlias = worldAlias;
        req.targetName = targetName;
        return postWorldAction(req);
    }

    public BackendResponse postWorldAction(WorldAction req) throws IOException {
        Map<String, String> kv = new LinkedHashMap<>();
        kv.put("action", req.action);
        kv.put("actor_uuid", req.actorUuid);
        kv.put("actor_name", req.actorName);
        kv.put("world_alias", req.worldAlias);
        kv.put("target_name", req.targetName);
        kv.put("game_version", req.gameVersion);
        kv.put("template_name", req.templateName);
        kv.put("reason", req.reason);
        kv.put("access_mode", req.accessMode);
        kv.put("request_id", req.requestId == null || req.requestId.trim().isEmpty() ? UUID.randomUUID().toString() : req.requestId);

        StringBuilder form = new StringBuilder();
        for (Map.Entry<String, String> e : kv.entrySet()) {
            if (e.getValue() == null || e.getValue().trim().isEmpty()) {
                continue;
            }
            if (form.length() > 0) {
                form.append("&");
            }
            form.append(enc(e.getKey())).append("=").append(enc(e.getValue()));
        }
        HttpURLConnection conn = (HttpURLConnection) new URL(baseUrl + "/v1/cmd/world").openConnection();
        return doPostForm(conn, form.toString());
    }

    public BackendResponse postTemplateList(String actorUuid, String actorName) throws IOException {
        String form = "action=" + enc("template_list")
                + "&actor_uuid=" + enc(actorUuid)
                + "&actor_name=" + enc(actorName)
                + "&request_id=" + enc(UUID.randomUUID().toString());
        HttpURLConnection conn = (HttpURLConnection) new URL(baseUrl + "/v1/cmd/world").openConnection();
        return doPostForm(conn, form);
    }

    public BackendResponse postPlayerJoin(String actorUuid, String actorName) throws IOException {
        String form = "actor_uuid=" + enc(actorUuid)
                + "&actor_name=" + enc(actorName);
        HttpURLConnection conn = (HttpURLConnection) new URL(baseUrl + "/v1/cmd/player/join").openConnection();
        return doPostForm(conn, form);
    }

    private BackendResponse doPostForm(HttpURLConnection conn, String form) throws IOException {
        conn.setRequestMethod("POST");
        conn.setConnectTimeout(timeoutMs);
        conn.setReadTimeout(timeoutMs);
        conn.setDoOutput(true);
        conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
        if (!token.trim().isEmpty()) {
            conn.setRequestProperty("Authorization", "Bearer " + token);
        }

        byte[] payload = form.getBytes(StandardCharsets.UTF_8);
        conn.setRequestProperty("Content-Length", String.valueOf(payload.length));
        OutputStream os = conn.getOutputStream();
        os.write(payload);
        os.flush();
        os.close();

        int code = conn.getResponseCode();
        Scanner scanner;
        if (code >= 200 && code < 400) {
            scanner = new Scanner(conn.getInputStream(), "UTF-8");
        } else if (conn.getErrorStream() != null) {
            scanner = new Scanner(conn.getErrorStream(), "UTF-8");
        } else {
            scanner = null;
        }

        String body = "";
        if (scanner != null) {
            scanner.useDelimiter("\\A");
            body = scanner.hasNext() ? scanner.next() : "";
            scanner.close();
        }
        conn.disconnect();

        return new BackendResponse(code, body);
    }

    private static String trimTrailingSlash(String input) {
        String s = input == null ? "" : input.trim();
        while (s.endsWith("/")) {
            s = s.substring(0, s.length() - 1);
        }
        return s;
    }

    private static String enc(String value) {
        try {
            return URLEncoder.encode(value == null ? "" : value, "UTF-8");
        } catch (UnsupportedEncodingException e) {
            // UTF-8 is always available in JVM; fallback for checked exception compatibility.
            return value == null ? "" : value;
        }
    }

    public static final class BackendResponse {
        private final int statusCode;
        private final String body;

        public BackendResponse(int statusCode, String body) {
            this.statusCode = statusCode;
            this.body = body;
        }

        public int statusCode() {
            return statusCode;
        }

        public String body() {
            return body;
        }
    }

    public static final class WorldAction {
        private final String action;
        private final String actorUuid;
        private final String actorName;

        private String worldAlias = "";
        private String targetName = "";
        private String requestId = "";
        private String gameVersion = "";
        private String templateName = "";
        private String reason = "";
        private String accessMode = "";

        public WorldAction(String action, String actorUuid, String actorName) {
            this.action = action;
            this.actorUuid = actorUuid;
            this.actorName = actorName;
        }

        public WorldAction worldAlias(String value) {
            this.worldAlias = value;
            return this;
        }

        public WorldAction targetName(String value) {
            this.targetName = value;
            return this;
        }

        public WorldAction requestId(String value) {
            this.requestId = value;
            return this;
        }

        public WorldAction gameVersion(String value) {
            this.gameVersion = value;
            return this;
        }

        public WorldAction templateName(String value) {
            this.templateName = value;
            return this;
        }

        public WorldAction reason(String value) {
            this.reason = value;
            return this;
        }

        public WorldAction accessMode(String value) {
            this.accessMode = value;
            return this;
        }
    }
}
