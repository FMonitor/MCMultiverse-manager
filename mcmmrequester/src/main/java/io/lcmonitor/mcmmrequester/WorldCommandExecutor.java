package io.lcmonitor.mcmmrequester;

import org.bukkit.command.Command;
import org.bukkit.command.CommandExecutor;
import org.bukkit.command.CommandSender;
import org.bukkit.command.TabCompleter;
import org.bukkit.entity.Player;
import org.bukkit.plugin.java.JavaPlugin;

import java.io.IOException;
import java.time.Instant;
import java.util.Arrays;
import java.util.Collections;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

public final class WorldCommandExecutor implements CommandExecutor, TabCompleter {
    private static final List<String> ROOT_SUBCOMMANDS = Arrays.asList("world", "confirm");
    private static final List<String> WORLD_OPERATIONS = Arrays.asList("add", "remove", "delete");
    private static final long DELETE_CONFIRM_TTL_SECONDS = 30;

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
        if ("confirm".equals(root)) {
            return handleConfirm(player);
        }

        if (!"world".equals(root) || args.length < 2) {
            sendUsage(sender);
            return true;
        }

        String worldOrAction = args[1].toLowerCase(Locale.ROOT);
        String action;
        String worldAlias = "";
        String target = "";

        if ("create".equals(worldOrAction)) {
            action = "create";
            if (args.length != 2) {
                sender.sendMessage("Usage: /mcmm world create");
                return true;
            }
            return dispatch(player, action, worldAlias, target, "创建世界请求已发送（待后端返回链接和身份码）");
        }

        worldAlias = args[1];
        if (args.length >= 3 && "delete".equalsIgnoreCase(args[2])) {
            if (args.length != 3) {
                sender.sendMessage("Usage: /mcmm world <world_alias> delete");
                return true;
            }
            PendingDelete pending = new PendingDelete(worldAlias, Instant.now().plusSeconds(DELETE_CONFIRM_TTL_SECONDS));
            pendingDeletes.put(player.getUniqueId(), pending);
            sender.sendMessage("[MCMM] 确认删除世界 \"" + worldAlias + "\" 吗？在30秒内输入 /mcmm confirm 确认。");
            return true;
        }

        if (args.length >= 5 &&
                ("add".equalsIgnoreCase(args[2]) || "remove".equalsIgnoreCase(args[2])) &&
                "user".equalsIgnoreCase(args[3])) {
            action = "member_" + args[2].toLowerCase(Locale.ROOT);
            target = args[4];
            if (target.trim().isEmpty()) {
                sender.sendMessage("Target user is required.");
                return true;
            }
            String successHint = "member_add".equals(action)
                    ? "已发送添加成员请求：" + target + " -> " + worldAlias
                    : "已发送移除成员请求：" + target + " -> " + worldAlias;
            return dispatch(player, action, worldAlias, target, successHint);
        }

        sender.sendMessage("Unknown command pattern.");
        sendUsage(sender);
        return true;
    }

    private boolean handleConfirm(Player player) {
        PendingDelete pending = pendingDeletes.get(player.getUniqueId());
        if (pending == null) {
            player.sendMessage("[MCMM] 当前没有待确认的删除请求。");
            return true;
        }
        if (Instant.now().isAfter(pending.expiresAt)) {
            pendingDeletes.remove(player.getUniqueId());
            player.sendMessage("[MCMM] 删除确认已过期，请重新执行 /mcmm world <world_alias> delete");
            return true;
        }

        pendingDeletes.remove(player.getUniqueId());
        return dispatch(player, "delete", pending.worldAlias, "", "已删除世界 \"" + pending.worldAlias + "\"（删除流程已提交）");
    }

    private boolean dispatch(Player player, String action, String worldAlias, String target, String successHint) {
        if (dryRun) {
            player.sendMessage("[MCMM][DryRun] " + successHint);
            player.sendMessage("[MCMM][DryRun] action=" + action
                    + ", actor=" + player.getName()
                    + ", world_alias=" + worldAlias
                    + ", target=" + target);
            return true;
        }

        try {
            BackendClient.BackendResponse response = backend.postWorldAction(
                    action,
                    player.getUniqueId().toString(),
                    player.getName(),
                    worldAlias,
                    target
            );
            player.sendMessage("[MCMM] status=" + response.statusCode());
            if (response.body() != null && !response.body().trim().isEmpty()) {
                player.sendMessage("[MCMM] " + response.body());
            }
            return true;
        } catch (IOException e) {
            plugin.getLogger().warning("backend request failed: " + e.getMessage());
            player.sendMessage("[MCMM] request failed: " + e.getMessage());
            return true;
        }
    }

    private static void sendUsage(CommandSender sender) {
        sender.sendMessage("Usage: /mcmm world create");
        sender.sendMessage("Usage: /mcmm world <world_alias> add user <user>");
        sender.sendMessage("Usage: /mcmm world <world_alias> remove user <user>");
        sender.sendMessage("Usage: /mcmm world <world_alias> delete");
        sender.sendMessage("Usage: /mcmm confirm");
    }

    @Override
    public List<String> onTabComplete(CommandSender sender, Command command, String alias, String[] args) {
        if (args.length == 1) {
            String prefix = args[0].toLowerCase(Locale.ROOT);
            List<String> out = new ArrayList<>();
            for (String c : ROOT_SUBCOMMANDS) {
                if (c.startsWith(prefix)) {
                    out.add(c);
                }
            }
            return out;
        }
        if (args.length == 2 && "world".equalsIgnoreCase(args[0])) {
            return Arrays.asList("create", "<world_alias>");
        }
        if (args.length == 3 && "world".equalsIgnoreCase(args[0])) {
            return WORLD_OPERATIONS;
        }
        if (args.length == 4 && "world".equalsIgnoreCase(args[0]) &&
                ("add".equalsIgnoreCase(args[2]) || "remove".equalsIgnoreCase(args[2]))) {
            return Collections.singletonList("user");
        }
        return Collections.emptyList();
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
