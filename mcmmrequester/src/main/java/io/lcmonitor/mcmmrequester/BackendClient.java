package io.lcmonitor.mcmmrequester;

import java.io.IOException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.UUID;

public final class BackendClient {
    private final String baseUrl;
    private final String token;
    private final HttpClient client;

    public BackendClient(String baseUrl, String token, int timeoutMs) {
        this.baseUrl = trimTrailingSlash(baseUrl);
        this.token = token == null ? "" : token.trim();
        int timeout = Math.max(timeoutMs, 1000);
        this.client = HttpClient.newBuilder()
                .connectTimeout(Duration.ofMillis(timeout))
                .build();
    }

    public BackendResponse postWorldAction(String action, String actorUuid, String actorName, String worldAlias, String targetName)
            throws IOException, InterruptedException {
        String form = "action=" + enc(action)
                + "&actor_uuid=" + enc(actorUuid)
                + "&actor_name=" + enc(actorName)
                + "&world_alias=" + enc(worldAlias)
                + "&target_name=" + enc(targetName == null ? "" : targetName)
                + "&request_id=" + enc(UUID.randomUUID().toString());

        HttpRequest.Builder reqBuilder = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + "/v1/cmd/world"))
                .timeout(Duration.ofSeconds(10))
                .header("Content-Type", "application/x-www-form-urlencoded")
                .POST(HttpRequest.BodyPublishers.ofString(form));

        if (!token.isBlank()) {
            reqBuilder.header("Authorization", "Bearer " + token);
        }

        HttpResponse<String> response = client.send(reqBuilder.build(), HttpResponse.BodyHandlers.ofString());
        return new BackendResponse(response.statusCode(), response.body());
    }

    private static String trimTrailingSlash(String input) {
        String s = input == null ? "" : input.trim();
        while (s.endsWith("/")) {
            s = s.substring(0, s.length() - 1);
        }
        return s;
    }

    private static String enc(String value) {
        return URLEncoder.encode(value == null ? "" : value, StandardCharsets.UTF_8);
    }

    public record BackendResponse(int statusCode, String body) {}
}
