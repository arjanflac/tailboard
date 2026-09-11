package com.arjanflac.tailboard;

/** Remembers one receipt ID, never clipboard text. Called on the main thread. */
final class ClipboardUpdates {
    private String lastID;

    ClipboardUpdates(String lastID) {
        this.lastID = lastID;
    }

    boolean apply(String id, Runnable write) {
        if (id.isEmpty() || id.equals(lastID)) return false;
        write.run();
        lastID = id;
        return true;
    }
}
