package com.arjanflac.tailboard;

import org.junit.Test;
import static org.junit.Assert.*;

public final class ClipboardUpdatesTest {
    @Test public void reconnectDoesNotOverwriteNewerPhoneText() {
        ClipboardUpdates updates = new ClipboardUpdates("");
        String[] clipboard = {""};
        assertTrue(updates.apply("mac-update-1", () -> clipboard[0] = "Mac text"));
        clipboard[0] = "New text copied on phone";
        for (int retry = 0; retry < 10; retry++) {
            assertFalse(updates.apply("mac-update-1", () -> clipboard[0] = "Mac text"));
        }
        assertEquals("New text copied on phone", clipboard[0]);
        assertTrue(updates.apply("mac-update-2", () -> clipboard[0] = "Fresh Mac text"));
        assertEquals("Fresh Mac text", clipboard[0]);
    }

    @Test public void savedReceiptSurvivesServiceRestart() {
        ClipboardUpdates restarted = new ClipboardUpdates("received-before-restart");
        assertFalse(restarted.apply("received-before-restart", () -> fail("Replayed clipboard")));
        assertTrue(restarted.apply("new-update", () -> {}));
    }

    @Test public void failedWriteCanBeRetried() {
        ClipboardUpdates updates = new ClipboardUpdates("");
        assertThrows(IllegalStateException.class, () -> updates.apply("new-update", () -> {
            throw new IllegalStateException("Clipboard unavailable");
        }));
        assertTrue(updates.apply("new-update", () -> {}));
    }

    @Test public void unidentifiableUpdatesDoNotWriteAutomatically() {
        assertFalse(new ClipboardUpdates("").apply("", () -> fail("Missing update ID")));
    }
}
