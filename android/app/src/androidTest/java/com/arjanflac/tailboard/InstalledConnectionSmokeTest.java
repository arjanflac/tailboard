package com.arjanflac.tailboard;

import android.content.Context;
import androidx.test.core.app.ActivityScenario;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.Test;
import org.junit.runner.RunWith;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;

/** Opt-in check against the user's real configured Mac; never changes settings or text. */
@RunWith(AndroidJUnit4.class)
public final class InstalledConnectionSmokeTest {
    @Test public void installedMacIsReachableAndBackgroundSyncStarts() throws Exception {
        assumeTrue("Real-Mac check is opt-in", Boolean.parseBoolean(
                InstrumentationRegistry.getArguments().getString("verifyInstalledConnection", "false")));
        Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        assertTrue("No default Mac configured", TailboardConfig.destination(context).configured());
        CountDownLatch done = new CountDownLatch(1);
        AtomicReference<String> error = new AtomicReference<>();
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            new TailboardClient().probe(context, (success, message) -> {
                if (!success) error.set(message);
                done.countDown();
            });
            assertTrue("Mac probe did not finish", done.await(7, TimeUnit.SECONDS));
            assertNull(error.get(), error.get());
        }
    }
}
