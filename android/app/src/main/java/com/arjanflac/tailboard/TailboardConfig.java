package com.arjanflac.tailboard;

import android.content.Context;
import android.content.SharedPreferences;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.Locale;
import java.util.UUID;

final class TailboardConfig {
    static final String DEFAULT_SERVER_URL = "http://your-mac:9437";
    private static final String PREFS = "tailboard";
    private static final String SERVER = "server_url";
    private static final String MACS = "saved_mac_urls";
    private static final String MAC_NAME = "mac_name:";
    private static final String NAME = "device_name";
    private static final String ID = "device_id";
    private static final String RECEIVED = "last_received_id";

    private TailboardConfig() {}

    private static SharedPreferences preferences(Context context) {
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    static String serverURL(Context context) {
        return preferences(context).getString(SERVER, DEFAULT_SERVER_URL);
    }

    static MacDestination destination(Context context) {
        String url = serverURL(context);
        return new MacDestination(preferences(context).getString(MAC_NAME + url, "Mac"), url);
    }

    static List<MacDestination> savedMacs(Context context) {
        SharedPreferences prefs = preferences(context);
        Set<String> urls = new HashSet<>(prefs.getStringSet(MACS, new HashSet<>()));
        if (destination(context).configured()) urls.add(serverURL(context));
        List<MacDestination> result = new ArrayList<>();
        for (String url : urls) result.add(new MacDestination(prefs.getString(MAC_NAME + url, "Mac"), url));
        result.sort((a, b) -> a.name.compareToIgnoreCase(b.name));
        return result;
    }

    static String lastReceivedID(Context context) {
        return preferences(context).getString(RECEIVED, "");
    }

    static void received(Context context, String id) {
        preferences(context).edit().putString(RECEIVED, id).apply();
    }

    static String deviceName(Context context) {
        return preferences(context).getString(NAME, "Android");
    }

    static String deviceID(Context context) {
        SharedPreferences preferences = preferences(context);
        String value = preferences.getString(ID, null);
        if (value == null) {
            value = UUID.randomUUID().toString().toLowerCase(Locale.ROOT);
            preferences.edit().putString(ID, value).apply();
        }
        return value;
    }

    static void save(Context context, String serverURL, String macName, String deviceName) {
        String normalized = MacDestination.normalizeAddress(serverURL);
        SharedPreferences prefs = preferences(context);
        Set<String> urls = new HashSet<>(prefs.getStringSet(MACS, new HashSet<>()));
        if (destination(context).configured()) urls.add(serverURL(context));
        urls.add(normalized);
        prefs.edit()
                .putStringSet(MACS, urls)
                .putString(SERVER, normalized)
                .putString(MAC_NAME + normalized, macName.trim())
                .putString(NAME, deviceName.trim())
                .apply();
    }
}
