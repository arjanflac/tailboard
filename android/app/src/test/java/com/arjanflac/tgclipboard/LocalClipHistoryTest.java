package com.arjanflac.tgclipboard;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;

import org.junit.Test;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

public final class LocalClipHistoryTest {
    @Test
    public void keepsNewestTwentyWithinTwentyFourHours() {
        long now = Instant.parse("2026-08-31T20:00:00Z").toEpochMilli();
        List<HubClient.Clip> clips = new ArrayList<>();
        for (int index = 1; index <= 25; index++) {
            clips.add(clip(index, now - (25L - index) * 60_000L));
        }
        clips.add(clip(26, now - 25L * 60L * 60L * 1_000L));

        List<HubClient.Clip> retained = LocalClipHistory.retained(clips, now);

        assertEquals(20, retained.size());
        assertEquals(25L, retained.get(0).sequence);
        assertEquals(6L, retained.get(19).sequence);
        assertFalse(retained.stream().anyMatch(clip -> clip.sequence == 26L));
    }

    @Test
    public void deduplicatesBySequence() {
        long now = Instant.parse("2026-08-31T20:00:00Z").toEpochMilli();
        HubClient.Clip duplicate = clip(2, now);
        List<HubClient.Clip> retained = LocalClipHistory.retained(
                List.of(duplicate, clip(1, now - 60_000L), duplicate),
                now
        );

        assertEquals(List.of(2L, 1L), retained.stream().map(clip -> clip.sequence).toList());
    }

    private static HubClient.Clip clip(long sequence, long createdAtMillis) {
        return new HubClient.Clip(
                sequence,
                "clip " + sequence,
                "test",
                "device",
                Instant.ofEpochMilli(createdAtMillis).toString()
        );
    }
}
