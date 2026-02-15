package io.lcmonitor.mcmmrequester;

import org.bukkit.command.PluginCommand;
import org.bukkit.plugin.java.JavaPlugin;

public final class MCMMRequesterPlugin extends JavaPlugin {
    private WorldCommandExecutor mcmmCommandExecutor;

    @Override
    public void onEnable() {
        saveDefaultConfig();

        String baseUrl = getConfig().getString("backend.base-url", "http://mcmm-api:8080");
        String token = getConfig().getString("backend.token", "");
        int timeoutMs = getConfig().getInt("backend.timeout-ms", 5000);
        boolean dryRun = getConfig().getBoolean("backend.dry-run", false);

        BackendClient backendClient = new BackendClient(baseUrl, token, timeoutMs);
        mcmmCommandExecutor = new WorldCommandExecutor(this, backendClient, dryRun);

        PluginCommand cmd = getCommand("mcmm");
        if (cmd == null) {
            getLogger().severe("/mcmm command not found in plugin.yml");
            getServer().getPluginManager().disablePlugin(this);
            return;
        }

        cmd.setExecutor(mcmmCommandExecutor);
        cmd.setTabCompleter(mcmmCommandExecutor);
        getServer().getPluginManager().registerEvents(new PlayerJoinListener(this, backendClient, dryRun), this);

        getLogger().info("MCMMRequester enabled. backend=" + baseUrl + ", dryRun=" + dryRun);
    }

    @Override
    public void onDisable() {
        getLogger().info("MCMMRequester disabled");
    }
}
