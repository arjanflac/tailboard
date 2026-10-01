package com.arjanflac.tailboard;

import java.nio.charset.StandardCharsets;
import okhttp3.HttpUrl;

/** One explicitly selected Tailboard server; never discovers or broadcasts to peers. */
final class MacDestination {
    final String name;
    final String url;

    MacDestination(String name, String url) {
        this.name = name == null || name.isBlank() ? "Mac" : name.trim();
        this.url = url;
    }

    static String normalizeAddress(String input) {
        String value = input.trim();
        if (value.isEmpty()) throw new IllegalArgumentException("Enter your Mac's Tailscale address");
        boolean explicitURL = value.contains("://");
        HttpUrl parsed = HttpUrl.parse(explicitURL ? value : "http://" + value);
        if (parsed == null || !parsed.username().isEmpty() || !parsed.password().isEmpty()
                || !parsed.encodedPath().equals("/") || parsed.query() != null
                || parsed.fragment() != null || parsed.host().equals("your-mac")) {
            throw new IllegalArgumentException("Enter a Tailscale IP, hostname, or Tailboard URL");
        }
        if (!explicitURL && !value.matches(".*:\\d+/?$")) parsed = parsed.newBuilder().port(9437).build();
        String result = parsed.toString();
        return result.substring(0, result.length() - 1);
    }

    boolean configured() {
        return url != null && !url.isBlank() && !url.equals("http://your-mac:9437");
    }

    String unavailable() {
        return name + " unavailable. Check Tailscale and Tailboard.";
    }

    String httpError(int code) {
        if (code == 404) return "Tailboard not found on " + name + ". Check its address.";
        if (code == 400 || code == 413) return "Couldn't send this clipboard text to " + name + ".";
        return "Tailboard on " + name + " returned an error (" + code + ").";
    }

    static boolean validText(String text) {
        return text != null && !text.isEmpty()
                && text.getBytes(StandardCharsets.UTF_8).length <= 1_048_576;
    }
}
