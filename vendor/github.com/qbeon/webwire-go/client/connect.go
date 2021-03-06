package client

import (
	"sync/atomic"
)

// connect will try to establish a connection to the configured webwire server
// and try to automatically restore the session if there is any.
// If the session restoration fails connect won't fail, instead it will reset the current session
// and return normally.
// Before establishing the connection - connect verifies protocol compatibility and returns an
// error if the protocol implemented by the server doesn't match the required protocol version
// of this client instance.
func (clt *Client) connect() error {
	clt.connectLock.Lock()
	defer clt.connectLock.Unlock()
	if atomic.LoadInt32(&clt.status) == StatConnected {
		return nil
	}

	if err := clt.verifyProtocolVersion(); err != nil {
		return err
	}

	if err := clt.conn.Dial(clt.serverAddr); err != nil {
		return err
	}

	// Setup reader thread
	go func() {
		defer clt.close()
		for {
			message, err := clt.conn.Read()
			if err != nil {
				if err.IsAbnormalCloseErr() {
					// Error while reading message
					clt.errorLog.Print("Abnormal closure error:", err)
				}

				// Set status to disconnected if it wasn't disabled
				if atomic.LoadInt32(&clt.status) == StatConnected {
					atomic.StoreInt32(&clt.status, StatDisconnected)
				}

				// Call hook
				clt.hooks.OnDisconnected()

				// Try to reconnect if the client wasn't disabled and autoconnect is on.
				// reconnect in another goroutine to let this one die and free up the socket
				go func() {
					if clt.autoconnect && atomic.LoadInt32(&clt.status) != StatDisabled {
						if err := clt.tryAutoconnect(0); err != nil {
							clt.errorLog.Printf("Auto-reconnect failed after connection loss: %s", err)
							return
						}
					}
				}()
				return
			}
			// Try to handle the message
			if err := clt.handleMessage(message); err != nil {
				clt.warningLog.Print("Failed handling message:", err)
			}
		}
	}()

	atomic.StoreInt32(&clt.status, StatConnected)

	// Read the current sessions key if there is any
	clt.sessionLock.RLock()
	if clt.session == nil {
		clt.sessionLock.RUnlock()
		return nil
	}
	sessionKey := clt.session.Key
	clt.sessionLock.RUnlock()

	// Try to restore session if necessary
	restoredSession, err := clt.requestSessionRestoration([]byte(sessionKey))
	if err != nil {
		// Just log a warning and still return nil, even if session restoration failed,
		// because we only care about the connection establishment in this method
		clt.warningLog.Printf("Couldn't restore session on reconnection: %s", err)

		// Reset the session
		clt.sessionLock.Lock()
		clt.session = nil
		clt.sessionLock.Unlock()
		return nil
	}

	clt.sessionLock.Lock()
	clt.session = restoredSession
	clt.sessionLock.Unlock()
	return nil
}
