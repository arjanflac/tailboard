package com.arjanflac.tailboard;

import android.content.Context;
import android.annotation.SuppressLint;
import android.content.BroadcastReceiver;
import android.content.IntentFilter;
import android.os.Build;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import androidx.test.core.app.ActivityScenario;
import android.content.Intent;
import android.content.SharedPreferences;
import androidx.test.ext.junit.runners.AndroidJUnit4;
import androidx.test.platform.app.InstrumentationRegistry;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import org.junit.runner.RunWith;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import static org.junit.Assert.*;

/** Runs on an actual phone; preserves its settings and never sends user clipboard text. */
@RunWith(AndroidJUnit4.class)
public final class DestinationDeviceTest {
    private Context context;
    private SharedPreferences prefs;
    private Map<String, ?> saved;

    @Before public void prepare() {
        context = InstrumentationRegistry.getInstrumentation().getTargetContext();
        prefs = context.getSharedPreferences("tailboard", Context.MODE_PRIVATE);
        saved = prefs.getAll();
        context.stopService(new Intent(context, ClipboardSyncService.class));
        prefs.edit().clear().commit();
    }

    @After @SuppressWarnings("unchecked") public void restore() {
        context.stopService(new Intent(context, ClipboardSyncService.class));
        SharedPreferences.Editor edit = prefs.edit().clear();
        for (Map.Entry<String, ?> entry : saved.entrySet()) {
            Object value = entry.getValue();
            if (value instanceof String) edit.putString(entry.getKey(), (String)value);
            else if (value instanceof Set) edit.putStringSet(entry.getKey(), (Set<String>)value);
        }
        edit.commit();
    }

    @Test public void upgradePreservesLegacyAddressAndSwitchesBetweenSavedMacs() {
        prefs.edit().putString("server_url", "http://100.64.1.2:9437").commit();
        assertEquals("http://100.64.1.2:9437", TailboardConfig.destination(context).url);
        assertEquals(1, TailboardConfig.savedMacs(context).size());
        TailboardConfig.save(context, "100.64.1.3", "Office Mac", "Pixel");
        assertEquals(2, TailboardConfig.savedMacs(context).size());
        TailboardConfig.save(context, "100.64.1.2", "MacBook", "Pixel");
        assertEquals("MacBook", TailboardConfig.destination(context).name);
        assertEquals("http://100.64.1.2:9437", TailboardConfig.serverURL(context));
        assertEquals(2, TailboardConfig.savedMacs(context).size());
    }

    @Test public void settingsSaveNamedDefaultAndKeepPreviousMac() {
        TailboardConfig.save(context, "100.64.1.2", "MacBook", "Pixel");
        try (ActivityScenario<SettingsActivity> scenario = ActivityScenario.launch(SettingsActivity.class)) {
            scenario.onActivity(activity -> {
                assertEquals("MacBook", ((EditText)activity.findViewById(R.id.mac_name)).getText().toString());
                ((EditText)activity.findViewById(R.id.mac_name)).setText("Office Mac");
                ((EditText)activity.findViewById(R.id.mac_address)).setText("100.64.1.3");
                Button save = findButton(activity.getWindow().getDecorView(), "Save as default Mac");
                assertNotNull(save);
                save.performClick();
            });
        }
        assertEquals("Office Mac", TailboardConfig.destination(context).name);
        assertEquals("http://100.64.1.3:9437", TailboardConfig.serverURL(context));
        assertEquals(2, TailboardConfig.savedMacs(context).size());
    }

    private static Button findButton(View view, String label) {
        if (view instanceof Button && label.contentEquals(((Button)view).getText())) return (Button)view;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup)view;
            for (int i = 0; i < group.getChildCount(); i++) {
                Button found = findButton(group.getChildAt(i), label);
                if (found != null) return found;
            }
        }
        return null;
    }

    @Test @SuppressLint("UnspecifiedRegisterReceiverFlag")
    public void quickSendServiceReportsNamedFailureAndAccurateNotification() throws Exception {
        int port;
        try (ServerSocket socket = new ServerSocket(0)) { port = socket.getLocalPort(); }
        TailboardConfig.save(context, "http://127.0.0.1:" + port, "Office Mac", "Pixel");
        CountDownLatch done = new CountDownLatch(1);
        BroadcastReceiver receiver = new BroadcastReceiver() {
            @Override public void onReceive(Context context, Intent intent) {
                if ("Office Mac unavailable. Check Tailscale and Tailboard.".equals(
                        intent.getStringExtra(ClipboardSyncService.EXTRA_STATUS))) done.countDown();
            }
        };
        IntentFilter filter = new IntentFilter(ClipboardSyncService.ACTION_STATUS);
        if (Build.VERSION.SDK_INT >= 33) context.registerReceiver(receiver, filter, Context.RECEIVER_NOT_EXPORTED);
        else context.registerReceiver(receiver, filter);
        try (ActivityScenario<MainActivity> scenario = ActivityScenario.launch(MainActivity.class)) {
            scenario.onActivity(activity -> ClipboardSyncService.send(activity, "synthetic test"));
            assertTrue("Quick send did not report failure", done.await(7, TimeUnit.SECONDS));
            android.app.NotificationManager manager = context.getSystemService(android.app.NotificationManager.class);
            for (android.service.notification.StatusBarNotification notification : manager.getActiveNotifications()) {
                if (notification.getId() == 9437) {
                    String text = notification.getNotification().extras.getString(android.app.Notification.EXTRA_TEXT);
                    assertNotNull(text);
                    assertFalse("Notification claimed an unavailable Mac was connected", text.startsWith("Connected"));
                }
            }
        } finally {
            context.unregisterReceiver(receiver);
        }
    }

    @Test public void unconfiguredPhoneRequestsDefaultMacInsteadOfSending() throws Exception {
        CountDownLatch done = new CountDownLatch(1);
        AtomicReference<String> result = new AtomicReference<>();
        new TailboardClient().postText(context, "synthetic test", (clip, error) -> {
            assertNull(clip);
            result.set(error);
            done.countDown();
        });
        assertTrue(done.await(1, TimeUnit.SECONDS));
        assertEquals("Choose a default Mac in Tailboard", result.get());
    }

    @Test public void unavailableMacReturnsNamedFailurePromptly() throws Exception {
        int port;
        try (ServerSocket socket = new ServerSocket(0)) { port = socket.getLocalPort(); }
        TailboardConfig.save(context, "http://127.0.0.1:" + port, "Office Mac", "Pixel");
        CountDownLatch done = new CountDownLatch(1);
        AtomicReference<String> result = new AtomicReference<>();
        new TailboardClient().postText(context, "synthetic test", (clip, error) -> {
            result.set(clip == null ? error : "unexpected success");
            done.countDown();
        });
        assertTrue("Failure took longer than the send timeout", done.await(7, TimeUnit.SECONDS));
        assertEquals("Office Mac unavailable. Check Tailscale and Tailboard.", result.get());
    }

    @Test public void sendUsesCapturedDestinationEvenIfDefaultChangesDuringRequest() throws Exception {
        try (ServerSocket server = new ServerSocket(0)) {
            TailboardConfig.save(context, "http://127.0.0.1:" + server.getLocalPort(), "MacBook", "Pixel");
            CountDownLatch requested = new CountDownLatch(1);
            CountDownLatch reply = new CountDownLatch(1);
            CountDownLatch done = new CountDownLatch(1);
            AtomicReference<String> path = new AtomicReference<>();
            Thread responder = new Thread(() -> {
                try (Socket socket = server.accept()) {
                    socket.setSoTimeout(5000);
                    BufferedReader reader = new BufferedReader(new InputStreamReader(socket.getInputStream(), StandardCharsets.UTF_8));
                    path.set(reader.readLine());
                    String line;
                    int length = 0;
                    while (!(line = reader.readLine()).isEmpty()) {
                        if (line.toLowerCase().startsWith("content-length:")) length = Integer.parseInt(line.substring(15).trim());
                    }
                    char[] body = new char[length];
                    int read = 0;
                    while (read < length) read += reader.read(body, read, length - read);
                    requested.countDown();
                    reply.await(5, TimeUnit.SECONDS);
                    String response = "{\"id\":\"test-id\",\"content\":\"synthetic test\",\"source\":\"Pixel\"}";
                    socket.getOutputStream().write(("HTTP/1.1 201 Created\r\nContent-Type: application/json\r\nContent-Length: " + response.length() + "\r\nConnection: close\r\n\r\n" + response).getBytes(StandardCharsets.UTF_8));
                } catch (Exception ignored) { requested.countDown(); }
            });
            responder.start();
            AtomicReference<TailboardClient.Clip> received = new AtomicReference<>();
            new TailboardClient().postText(context, "synthetic test", (clip, error) -> {
                received.set(clip);
                done.countDown();
            });
            assertTrue(requested.await(3, TimeUnit.SECONDS));
            TailboardConfig.save(context, "100.64.1.9", "Other Mac", "Pixel");
            reply.countDown();
            assertTrue(done.await(3, TimeUnit.SECONDS));
            assertEquals("POST /api/clip HTTP/1.1", path.get());
            assertNotNull(received.get());
            assertEquals("synthetic test", received.get().content);
            assertEquals("Other Mac", TailboardConfig.destination(context).name);
            responder.join(1000);
        }
    }
}
