package io.lcmonitor.mcmmrequester;

import org.bukkit.Bukkit;
import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.command.TabCompleter;
import org.bukkit.entity.Player;
import org.bukkit.plugin.java.JavaPlugin;

import java.io.IOException;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public final class WorldCommandExecutor implements CommandExecutor, TabCompleter {
    private static final List<String> ROOT_SUBCOMMANDS = Arrays.asList("req", "world", "template", "instance", "confirm", "help");
    private static final long DELETE_CONFIRM_TTL_SECONDS = 30;
    private static final Pattern MESSAGE_PATTERN = Pattern.compile("\"message\"\\s*:\\s*\"((?:\\\\.|[^\"])*)\"");

    private final JavaPlugin plugin;
    private final BackendClient backend;
    private final boolean dryRun;
    private final Map<UUID, PendingDelete> pendingDeletes = new ConcurrentHashMap<>();

    public WorldCommandExecutor(JavaPlugin plugin, BackendClient backend, boolean dryRun) {
        this.plugin = plugin;
        this.backend = backend;
        this.dryRun = dryRun;
    }

    @Override
    public boolean onCommand(CommandSender sender, Command command, String label, String[] args) {
        if (!(sender instanceof Player)) {
            sender.sendMessage("Only players can use this command.");
            return true;
        }
        Player player = (Player) sender;

        if (args.length < 1) {
            sendUsage(sender);
            return true;
        }

        String root = args[0].toLowerCase(Locale.ROOT);
        switch (root) {
            case "help":
                sendUsage(sender);
                return true;
            case "confirm":
                return handleConfirm(player);
            case "req":
                return handleReq(player, args);
            case "template":
                return handleTemplate(player, args);
            case "world":
                return handleWorld(player, args);
            case "instance":
                return handleInstance(player, args);
            default:
                sendUsage(sender);
                return true;
        }
    }

    private boolean handleTemplate(Player player, String[] args) {
        if (args.length == 2 && "list".equalsIgnoreCase(args[1])) {
            return dispatch(player, new BackendClient.WorldAction("template_list", player.getUniqueId().toString(), player.getName()), "template list");
        }
        player.sendMessage("Usage: /mcmm template list");
        return true;
    }

    private boolean handleReq(Player player, String[] args) {
        if (args.length < 2) {
            player.sendMessage("Usage: /mcmm req <create|list|approve|reject|cancel> ...");
            return true;
        }
        String op = args[1].toLowerCase(Locale.ROOT);
        switch (op) {
            case "create":
                if (args.length < 3 || args.length > 4) {
                    player.sendMessage("Usage: /mcmm req create <world_alias> [template_id|template_name]");
                    return true;
                }
                BackendClient.WorldAction action = new BackendClient.WorldAction("request_create", player.getUniqueId().toString(), player.getName())
                        .worldAlias(args[2]);
                if (args.length == 4) {
                    action.templateName(args[3]);
                }
                return dispatch(player, action, "request create");
            case "list":
                return dispatch(player,
                        new BackendClient.WorldAction("request_list", player.getUniqueId().toString(), player.getName()),
                        "request list");
            case "approve":
                if (args.length != 3) {
                    player.sendMessage("Usage: /mcmm req approve <request_no|request_id>");
                    return true;
                }
                return dispatch(player,
                        new BackendClient.WorldAction("request_approve", player.getUniqueId().toString(), player.getName())
                                .requestId(args[2]),
                        "request approve");
            case "reject":
                if (args.length < 3) {
                    player.sendMessage("Usage: /mcmm req reject <request_no|request_id> [reason]");
                    return true;
                }
                return dispatch(player,
                        new BackendClient.WorldAction("request_reject", player.getUniqueId().toString(), player.getName())
                                .requestId(args[2])
                                .reason(joinTail(args, 3)),
                        "request reject");
            case "cancel":
                if (args.length < 3) {
                    player.sendMessage("Usage: /mcmm req cancel <request_no|request_id> [reason]");
                    return true;
                }
                return dispatch(player,
                        new BackendClient.WorldAction("request_cancel", player.getUniqueId().toString(), player.getName())
                                .requestId(args[2])
                                .reason(joinTail(args, 3)),
                        "request cancel");
            default:
                player.sendMessage("Unsupported req action.");
                return true;
        }
    }

    private boolean handleWorld(Player player, String[] args) {
        if (args.length < 2) {
            sendUsage(player);
            return true;
        }

        String sub = args[1].toLowerCase(Locale.ROOT);
        if ("list".equals(sub)) {
            return dispatch(player, new BackendClient.WorldAction("world_list", player.getUniqueId().toString(), player.getName()), "world list");
        }
        if ("info".equals(sub)) {
            BackendClient.WorldAction action = new BackendClient.WorldAction("world_info", player.getUniqueId().toString(), player.getName());
            if (args.length >= 3) {
                action.worldAlias(args[2]);
            }
            return dispatch(player, action, "world info");
        }
        if ("set".equals(sub)) {
            if (args.length != 3 || (!"public".equalsIgnoreCase(args[2]) && !"privacy".equalsIgnoreCase(args[2]))) {
                player.sendMessage("Usage: /mcmm world set <public|privacy>");
                return true;
            }
            return dispatch(player,
                    new BackendClient.WorldAction("world_set_access", player.getUniqueId().toString(), player.getName())
                            .accessMode(args[2].toLowerCase(Locale.ROOT)),
                    "world set access");
        }
        if ("remove".equals(sub)) {
            if (args.length != 3) {
                player.sendMessage("Usage: /mcmm world remove <instance_id|alias>");
                return true;
            }
            PendingDelete pending = new PendingDelete(args[2], Instant.now().plusSeconds(DELETE_CONFIRM_TTL_SECONDS));
            pendingDeletes.put(player.getUniqueId(), pending);
            player.sendMessage("[MCMM] Confirm remove \"" + args[2] + "\" in 30s: /mcmm confirm");
            return true;
        }
        if (args.length == 2) {
            return dispatch(player,
                    new BackendClient.WorldAction("world_join", player.getUniqueId().toString(), player.getName())
                            .worldAlias(args[1]),
                    "world join");
        }

        // /mcmm world <alias> add/remove user <name>
        if (args.length >= 5 &&
                ("add".equalsIgnoreCase(args[2]) || "remove".equalsIgnoreCase(args[2])) &&
                "user".equalsIgnoreCase(args[3])) {
            String action = "add".equalsIgnoreCase(args[2]) ? "member_add" : "member_remove";
            return dispatch(player,
                    new BackendClient.WorldAction(action, player.getUniqueId().toString(), player.getName())
                            .worldAlias(args[1])
                            .targetName(args[4]),
                    action);
        }

        sendUsage(player);
        return true;
    }

    private boolean handleInstance(Player player, String[] args) {
        if (args.length == 2 && "list".equalsIgnoreCase(args[1])) {
            return dispatch(player,
                    new BackendClient.WorldAction("instance_list", player.getUniqueId().toString(), player.getName()),
                    "instance list");
        }
        if (args.length >= 3 && "create".equalsIgnoreCase(args[1])) {
            BackendClient.WorldAction action = new BackendClient.WorldAction("instance_create", player.getUniqueId().toString(), player.getName())
                    .worldAlias(args[2]);
            if (args.length >= 4) {
                action.templateName(args[3]);
            }
            return dispatch(player, action, "instance create");
        }
        if (args.length == 3 && "remove".equalsIgnoreCase(args[1])) {
            return dispatch(player,
                    new BackendClient.WorldAction("instance_remove", player.getUniqueId().toString(), player.getName())
                            .worldAlias(args[2]),
                    "instance remove");
        }
        if (args.length == 3 && "stop".equalsIgnoreCase(args[1])) {
            return dispatch(player,
                    new BackendClient.WorldAction("instance_stop", player.getUniqueId().toString(), player.getName())
                            .worldAlias(args[2]),
                    "instance stop");
        }
        if (args.length == 3 && "lockdown".equalsIgnoreCase(args[1])) {
            return dispatch(player,
                    new BackendClient.WorldAction("instance_lockdown", player.getUniqueId().toString(), player.getName())
                            .worldAlias(args[2]),
                    "instance lockdown");
        }
        player.sendMessage("Usage: /mcmm instance <list|create|stop|remove|lockdown> ...");
        return true;
    }

    private boolean handleConfirm(Player player) {
        PendingDelete pending = pendingDeletes.get(player.getUniqueId());
        if (pending == null) {
            player.sendMessage("[MCMM] No pending remove confirmation.");
            return true;
        }
        if (Instant.now().isAfter(pending.expiresAt)) {
            pendingDeletes.remove(player.getUniqueId());
            player.sendMessage("[MCMM] Remove confirmation expired.");
            return true;
        }
        pendingDeletes.remove(player.getUniqueId());
        return dispatch(player,
                new BackendClient.WorldAction("world_remove", player.getUniqueId().toString(), player.getName())
                        .worldAlias(pending.worldAlias),
                "world remove");
    }

    private boolean dispatch(Player player, BackendClient.WorldAction action, String summary) {
        if (dryRun) {
            player.sendMessage("[MCMM][DryRun] " + summary);
            return true;
        }
        player.sendMessage("[MCMM] processing...");
        Bukkit.getScheduler().runTaskAsynchronously(plugin, () -> {
            try {
                BackendClient.BackendResponse response = backend.postWorldAction(action);
                Bukkit.getScheduler().runTask(plugin, () -> {
                    String msg = extractBackendMessage(response.body());
                    if (response.statusCode() >= 200 && response.statusCode() < 300) {
                        player.sendMessage("[MCMM] " + (msg.isEmpty() ? "ok" : msg));
                        return;
                    }
                    if (msg.isEmpty()) {
                        msg = response.body();
                    }
                    player.sendMessage("[MCMM] failed(" + response.statusCode() + "): " + msg);
                });
            } catch (IOException e) {
                plugin.getLogger().warning("backend request failed: " + e.getMessage());
                Bukkit.getScheduler().runTask(plugin, () ->
                        player.sendMessage("[MCMM] request failed: " + e.getMessage()));
            }
        });
        return true;
    }

    private static String joinTail(String[] args, int fromIndex) {
        if (args.length <= fromIndex) {
            return "";
        }
        StringBuilder sb = new StringBuilder();
        for (int i = fromIndex; i < args.length; i++) {
            if (sb.length() > 0) {
                sb.append(" ");
            }
            sb.append(args[i]);
        }
        return sb.toString();
    }

    private static String extractBackendMessage(String body) {
        if (body == null) {
            return "";
        }
        String trimmed = body.trim();
        if (trimmed.isEmpty()) {
            return "";
        }
        Matcher m = MESSAGE_PATTERN.matcher(trimmed);
        if (!m.find()) {
            return trimmed;
        }
        String msg = m.group(1);
        return msg
                .replace("\\\"", "\"")
                .replace("\\\\", "\\")
                .replace("\\n", " ")
                .replace("\\r", " ")
                .trim();
    }

    private static void sendUsage(CommandSender sender) {
        sender.sendMessage("=== MCMM Commands ===");
        sender.sendMessage("Usage: /mcmm req create <world_alias> [template_id|template_name]");
        sender.sendMessage("Usage: /mcmm req list");
        sender.sendMessage("Usage: /mcmm req approve <request_no|request_id>");
        sender.sendMessage("Usage: /mcmm req reject <request_no|request_id> [reason]");
        sender.sendMessage("Usage: /mcmm req cancel <request_no|request_id> [reason]");
        sender.sendMessage("Usage: /mcmm world remove <instance_id|alias>");
        sender.sendMessage("Usage: /mcmm world <instance_id|alias>    # join world");
        sender.sendMessage("Usage: /mcmm world list");
        sender.sendMessage("Usage: /mcmm world set <public|privacy>");
        sender.sendMessage("Usage: /mcmm world info [instance_id|alias]");
        sender.sendMessage("Usage: /mcmm world <world_alias> add user <user>");
        sender.sendMessage("Usage: /mcmm world <world_alias> remove user <user>");
        sender.sendMessage("Usage: /mcmm template list");
        sender.sendMessage("Usage: /mcmm instance list");
        sender.sendMessage("Usage: /mcmm instance create <world_alias> [template_id|template_name]");
        sender.sendMessage("Usage: /mcmm instance stop <instance_id|alias>");
        sender.sendMessage("Usage: /mcmm instance remove <instance_id|alias>");
        sender.sendMessage("Usage: /mcmm instance lockdown <instance_id|alias>");
        sender.sendMessage("Usage: /mcmm confirm");
        sender.sendMessage("Usage: /mcmm help");
    }

    @Override
    public List<String> onTabComplete(CommandSender sender, Command command, String alias, String[] args) {
        boolean adminView = hasAdminView(sender);
        if (args.length == 1) {
            if (adminView) {
                return prefixMatch(ROOT_SUBCOMMANDS, args[0]);
            }
            return prefixMatch(Arrays.asList("req", "world", "template", "confirm", "help"), args[0]);
        }
        if ("req".equalsIgnoreCase(args[0]) && args.length == 2) {
            if (adminView) {
                return prefixMatch(Arrays.asList("create", "list", "approve", "reject", "cancel"), args[1]);
            }
            return prefixMatch(Arrays.asList("create", "list", "cancel"), args[1]);
        }
        if ("req".equalsIgnoreCase(args[0]) && args.length == 4 && "create".equalsIgnoreCase(args[1])) {
            return prefixMatch(Collections.singletonList("<template_id|template_name>"), args[3]);
        }
        if ("template".equalsIgnoreCase(args[0]) && args.length == 2) {
            return prefixMatch(Collections.singletonList("list"), args[1]);
        }
        if ("instance".equalsIgnoreCase(args[0]) && args.length == 2 && adminView) {
            return prefixMatch(Arrays.asList("list", "create", "stop", "remove", "lockdown"), args[1]);
        }
        if ("instance".equalsIgnoreCase(args[0]) && args.length == 4 && "create".equalsIgnoreCase(args[1]) && adminView) {
            return prefixMatch(Collections.singletonList("<template_id|template_name>"), args[3]);
        }
        if ("world".equalsIgnoreCase(args[0]) && args.length == 2) {
            return prefixMatch(Arrays.asList("list", "info", "set", "remove", "<world_alias>"), args[1]);
        }
        if ("world".equalsIgnoreCase(args[0]) && args.length == 3 && "set".equalsIgnoreCase(args[1])) {
            return prefixMatch(Arrays.asList("public", "privacy"), args[2]);
        }
        if ("world".equalsIgnoreCase(args[0]) && args.length == 3 && !isKeyword(args[1])) {
            return prefixMatch(Arrays.asList("add", "remove"), args[2]);
        }
        if ("world".equalsIgnoreCase(args[0]) && args.length == 4 &&
                ("add".equalsIgnoreCase(args[2]) || "remove".equalsIgnoreCase(args[2]))) {
            return prefixMatch(Collections.singletonList("user"), args[3]);
        }
        return Collections.emptyList();
    }

    private static boolean isKeyword(String s) {
        String k = s.toLowerCase(Locale.ROOT);
        return "list".equals(k) || "info".equals(k) || "set".equals(k) || "remove".equals(k);
    }

    private static List<String> prefixMatch(List<String> candidates, String rawPrefix) {
        String prefix = rawPrefix == null ? "" : rawPrefix.toLowerCase(Locale.ROOT);
        List<String> out = new ArrayList<>();
        for (String c : candidates) {
            if (c.toLowerCase(Locale.ROOT).startsWith(prefix)) {
                out.add(c);
            }
        }
        return out;
    }

    private static boolean hasAdminView(CommandSender sender) {
        return sender.isOp() || sender.hasPermission("mcmmrequester.admin");
    }

    private static final class PendingDelete {
        private final String worldAlias;
        private final Instant expiresAt;

        private PendingDelete(String worldAlias, Instant expiresAt) {
            this.worldAlias = worldAlias;
            this.expiresAt = expiresAt;
        }
    }
}
