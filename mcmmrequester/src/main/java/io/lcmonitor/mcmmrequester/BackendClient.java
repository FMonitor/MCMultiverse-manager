package io.lcmonitor.mcmmrequester;

import java.io.IOException;
import java.io.OutputStream;
import java.io.UnsupportedEncodingException;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
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
        String form = "action=" + enc(action)
                + "&actor_uuid=" + enc(actorUuid)
                + "&actor_name=" + enc(actorName)
                + "&world_alias=" + enc(worldAlias)
                + "&target_name=" + enc(targetName == null ? "" : targetName)
                + "&request_id=" + enc(UUID.randomUUID().toString());

        HttpURLConnection conn = (HttpURLConnection) new URL(baseUrl + "/v1/cmd/world").openConnection();
        return doPostForm(conn, form);
    }

    public BackendResponse postTemplateList(String actorUuid, String actorName) throws IOException {
        String form = "action=" + enc("template_list")
                + "&actor_uuid=" + enc(actorUuid)
                + "&actor_name=" + enc(actorName)
                + "&request_id=" + enc(UUID.randomUUID().toString());
        HttpURLConnection conn = (HttpURLConnection) new URL(baseUrl + "/v1/cmd/world").openConnection();
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
}
