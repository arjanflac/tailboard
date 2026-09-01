package com.arjanflac.tgclipboard;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONArray;
import org.json.JSONObject;

import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** Device-local recent clips. The Mac relay persists only its current value. */
final class LocalClipHistory {
    static final int MAX_ITEMS = 20;
    static final long MAX_AGE_MILLIS = 24L * 60L * 60L * 1_000L;

    private static final String KEY = "local_clip_history";

    private LocalClipHistory() {}

    static synchronized List<HubClient.Clip> load(Context context) {
        SharedPreferences preferences = HubConfig.preferences(context);
        String encoded = preferences.getString(KEY, "[]");
        List<HubClient.Clip> decoded = new ArrayList<>();
        try {
            JSONArray array = new JSONArray(encoded);
            for (int index = 0; index < array.length(); index++) {
                decoded.add(HubClient.Clip.fromJSON(array.getJSONObject(index)));
            }
        } catch (Exception ignored) {
            // A corrupt cache is disposable; connection settings live elsewhere.
        }
        List<HubClient.Clip> retained = retained(decoded, System.currentTimeMillis());
        if (retained.size() != decoded.size()) save(context, retained);
        return retained;
    }

    static synchronized List<HubClient.Clip> add(Context context, HubClient.Clip clip) {
        List<HubClient.Clip> clips = load(context);
        clips.add(0, clip);
        List<HubClient.Clip> retained = retained(clips, System.currentTimeMillis());
        save(context, retained);
        return retained;
    }

    static synchronized void clear(Context context) {
        HubConfig.preferences(context).edit().remove(KEY).apply();
    }

    static List<HubClient.Clip> retained(List<HubClient.Clip> clips, long nowMillis) {
        List<HubClient.Clip> sorted = new ArrayList<>(clips);
        sorted.sort(Comparator.comparingLong((HubClient.Clip clip) -> clip.sequence).reversed());
        long cutoff = nowMillis - MAX_AGE_MILLIS;
        Set<Long> seen = new HashSet<>();
        List<HubClient.Clip> retained = new ArrayList<>(MAX_ITEMS);
        for (HubClient.Clip clip : sorted) {
            if (retained.size() == MAX_ITEMS) break;
            if (!seen.add(clip.sequence) || createdAtMillis(clip) < cutoff) continue;
            retained.add(clip);
        }
        return retained;
    }

    private static void save(Context context, List<HubClient.Clip> clips) {
        JSONArray array = new JSONArray();
        for (HubClient.Clip clip : clips) {
            JSONObject item = new JSONObject();
            try {
                item.put("seq", clip.sequence);
                item.put("content", clip.content);
                item.put("source", clip.source);
                item.put("device_id", clip.deviceID);
                item.put("created_at", clip.createdAt);
                array.put(item);
            } catch (Exception ignored) {
                // Skip an item that cannot be serialized; the cache is best effort.
            }
        }
        HubConfig.preferences(context).edit().putString(KEY, array.toString()).apply();
    }

    private static long createdAtMillis(HubClient.Clip clip) {
        try {
            return Instant.parse(clip.createdAt).toEpochMilli();
        } catch (DateTimeParseException ignored) {
            return 0L;
        }
    }
}
