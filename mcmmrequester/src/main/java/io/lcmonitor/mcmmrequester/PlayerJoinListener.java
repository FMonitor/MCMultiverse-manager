package io.lcmonitor.mcmmrequester;

import org.bukkit.entity.Player;
import org.bukkit.event.EventHandler;
import org.bukkit.event.Listener;
import org.bukkit.event.player.PlayerJoinEvent;
import org.bukkit.plugin.java.JavaPlugin;
import org.bukkit.Bukkit;

import java.io.IOException;

public final class PlayerJoinListener implements Listener {
    private final JavaPlugin plugin;
    private final BackendClient backend;
    private final boolean dryRun;

    public PlayerJoinListener(JavaPlugin plugin, BackendClient backend, boolean dryRun) {
        this.plugin = plugin;
        this.backend = backend;
        this.dryRun = dryRun;
    }

    @EventHandler
    public void onPlayerJoin(PlayerJoinEvent event) {
        Player p = event.getPlayer();
        if (dryRun) {
            plugin.getLogger().info("[DryRun] skip player join sync for " + p.getName());
            return;
        }
        Bukkit.getScheduler().runTaskAsynchronously(plugin, () -> {
            try {
                BackendClient.BackendResponse resp = backend.postPlayerJoin(p.getUniqueId().toString(), p.getName());
                if (resp.statusCode() >= 300) {
                    plugin.getLogger().warning("player join sync failed status=" + resp.statusCode() + " body=" + resp.body());
                }
            } catch (IOException e) {
                plugin.getLogger().warning("player join sync error: " + e.getMessage());
            }
        });
    }
}
