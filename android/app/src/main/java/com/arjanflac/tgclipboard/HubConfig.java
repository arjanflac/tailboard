package com.arjanflac.tgclipboard;

import android.content.Context;
import android.content.SharedPreferences;

import java.util.Locale;
import java.util.UUID;

final class HubConfig {
    static final String DEFAULT_HUB_URL = "http://tailboard-hub:9437";
    static final String DEFAULT_DEVICE_NAME = "android";

    private static final String PREFS = "tg_clipboard";
    private static final String KEY_HUB_URL = "hub_url";
    private static final String KEY_DEVICE_NAME = "device_name";
    private static final String KEY_DEVICE_ID = "device_id";
    private static final String KEY_LAST_SEQUENCE = "last_sequence";

    private HubConfig() {}

    static SharedPreferences preferences(Context context) {
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    static String hubURL(Context context) {
        return preferences(context).getString(KEY_HUB_URL, DEFAULT_HUB_URL);
    }

    static String deviceName(Context context) {
        return preferences(context).getString(KEY_DEVICE_NAME, DEFAULT_DEVICE_NAME);
    }

    static String deviceID(Context context) {
        SharedPreferences preferences = preferences(context);
        String existing = preferences.getString(KEY_DEVICE_ID, null);
        if (existing != null && !existing.isBlank()) {
            return existing;
        }
        String generated = UUID.randomUUID().toString().toLowerCase(Locale.ROOT);
        preferences.edit().putString(KEY_DEVICE_ID, generated).apply();
        return generated;
    }

    static long lastSequence(Context context) {
        return preferences(context).getLong(KEY_LAST_SEQUENCE, 0L);
    }

    static void setLastSequence(Context context, long sequence) {
        preferences(context).edit().putLong(KEY_LAST_SEQUENCE, sequence).apply();
    }

    static void save(Context context, String hubURL, String deviceName) {
        String normalized = hubURL.trim();
        while (normalized.endsWith("/")) {
            normalized = normalized.substring(0, normalized.length() - 1);
        }
        preferences(context).edit()
                .putString(KEY_HUB_URL, normalized)
                .putString(KEY_DEVICE_NAME, deviceName.trim())
                .apply();
    }

    static void removeLegacyTransferSettings(Context context) {
        preferences(context).edit().remove("default_transfer_device").apply();
    }
}
