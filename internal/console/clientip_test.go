package console

import "testing"

func TestBlocksHyperlinkSameMachineClient(t *testing.T) {
	origViaOwnServer, origClientIP := viaOwnServer.Load(), clientIP
	defer func() {
		viaOwnServer.Store(origViaOwnServer)
		SetClientIP(origClientIP)
	}()

	SetViaOwnServer(true)

	SetClientIP("127.0.0.1")
	if blocksHyperlink() {
		t.Error("loopback client IP should not block hyperlinks even via own server")
	}

	SetClientIP("203.0.113.5") // TEST-NET-3, guaranteed not to match any local interface
	if !blocksHyperlink() {
		t.Error("a genuinely remote client IP via own server should block hyperlinks")
	}

	SetClientIP("")
	if !blocksHyperlink() {
		t.Error("an unknown client IP via own server should block hyperlinks")
	}
}

func TestBlocksHyperlinkNotViaOwnServer(t *testing.T) {
	origViaOwnServer, origClientIP := viaOwnServer.Load(), clientIP
	defer func() {
		viaOwnServer.Store(origViaOwnServer)
		SetClientIP(origClientIP)
	}()

	SetViaOwnServer(false)
	SetClientIP("203.0.113.5")
	if blocksHyperlink() {
		t.Error("sessions not via DS2's own server should never block hyperlinks")
	}
}
