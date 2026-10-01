package com.arjanflac.tailboard;

import org.junit.Test;
import static org.junit.Assert.*;

public final class MacDestinationTest {
    @Test public void bareTailscaleAddressesUseTailboardPort() {
        assertEquals("http://100.64.1.2:9437", MacDestination.normalizeAddress(" 100.64.1.2 "));
        assertEquals("http://macbook:9437", MacDestination.normalizeAddress("macbook"));
        assertEquals("http://macbook.tailnet.ts.net:9437", MacDestination.normalizeAddress("macbook.tailnet.ts.net"));
        assertEquals("http://macbook", MacDestination.normalizeAddress("macbook:80"));
        assertEquals("http://macbook:9440", MacDestination.normalizeAddress("macbook:9440"));
    }

    @Test public void explicitURLsPreserveCustomPortsAndHTTPS() {
        assertEquals("https://macbook", MacDestination.normalizeAddress("https://macbook/"));
        assertEquals("http://macbook:9437", MacDestination.normalizeAddress("http://macbook:9437/"));
    }

    @Test public void rejectsCredentialsPathsAndPlaceholder() {
        for (String value : new String[]{"", "http://user:password@macbook", "macbook/api/clip", "macbook?q=x", "macbook#x", "http://your-mac:9437", "ftp://macbook"}) {
            assertThrows(value, IllegalArgumentException.class, () -> MacDestination.normalizeAddress(value));
        }
    }

    @Test public void failuresIdentifyTheSelectedMacWithoutClaimingItIsPoweredOff() {
        MacDestination target = new MacDestination("Office Mac", "http://100.64.1.2:9437");
        assertEquals("Office Mac unavailable. Check Tailscale and Tailboard.", target.unavailable());
        assertTrue(target.httpError(404).contains("Office Mac"));
        assertFalse(new MacDestination("Mac", "http://your-mac:9437").configured());
    }

    @Test public void textLimitCountsUTF8Bytes() {
        assertTrue(MacDestination.validText("x".repeat(1_048_576)));
        assertFalse(MacDestination.validText("x".repeat(1_048_577)));
        assertFalse(MacDestination.validText("🙂".repeat(262_145)));
        assertFalse(MacDestination.validText(""));
    }
}
