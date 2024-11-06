// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sendfiletest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
)

const (
	zeroLen    = 1_073_741_824
	zeroSHA256 = "49bc20df15e412a64472421e13fe86ff1c5165e18b2afccf160d4dc19fe68a14"
)

func createZeroFile(t testing.TB, size int64) *os.File {
	f, err := os.CreateTemp("", "")
	if err != nil {
		t.Helper()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	err = f.Truncate(size)
	if err != nil {
		t.Helper()
		t.Fatal(err)
	}

	return f
}

func newLocalListener(t testing.TB) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Helper()
		t.Fatal(err)
	}

	return ln
}

func TestSendfile(t *testing.T) {
	f := createZeroFile(t, zeroLen)
	defer f.Close()

	ln := newLocalListener(t)
	defer ln.Close()

	errc := make(chan error, 1)
	go func(ln net.Listener) {
		// Wait for a connection.
		conn, err := ln.Accept()
		if err != nil {
			errc <- err
			close(errc)
			return
		}

		go func() {
			defer close(errc)
			defer conn.Close()

			// f := io.NopCloser(f) prevent sendfile usage

			// Return file data using io.ReaderFrom / io.Copy, which should use
			// sendFile if available.
			var sbytes int64
			if tcpconn, ok := conn.(*net.TCPConn); ok {
				// Shortcut for debugging
				sbytes, err = tcpconn.ReadFrom(f)
			} else {
				sbytes, err = io.Copy(conn, f)
			}
			if err != nil {
				errc <- err
				return
			}

			if sbytes != zeroLen {
				errc <- fmt.Errorf("sent %d bytes; expected %d", sbytes, zeroLen)
				return
			}
		}()
	}(ln)

	// Connect to listener to retrieve file and verify digest matches
	// expected.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	h := sha256.New()
	rbytes, err := io.Copy(h, c)
	if err != nil {
		t.Error(err)
	}

	if rbytes != zeroLen {
		t.Errorf("received %d bytes; expected %d", rbytes, zeroLen)
	}

	if res := hex.EncodeToString(h.Sum(nil)); res != zeroSHA256 {
		t.Error("retrieved data hash did not match")
	}

	for err := range errc {
		t.Error(err)
	}
}
