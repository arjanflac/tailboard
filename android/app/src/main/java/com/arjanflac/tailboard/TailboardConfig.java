package com.arjanflac.tailboard;

import android.content.Context;
import android.content.SharedPreferences;

import java.util.Locale;
import java.util.UUID;

final class TailboardConfig {
    static final String DEFAULT_SERVER_URL = "http://your-mac:9437";
    private static final String PREFS = "tailboard";
    private static final String SERVER = "server_url";
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

    static void save(Context context, String serverURL, String deviceName) {
        String normalized = serverURL.trim();
        while (normalized.endsWith("/")) {
            normalized = normalized.substring(0, normalized.length() - 1);
        }
        preferences(context).edit()
                .putString(SERVER, normalized)
                .putString(NAME, deviceName.trim())
                .apply();
    }
}
