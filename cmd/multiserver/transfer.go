package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CaptureState tracks an in-progress file download capture from a client.
// The recvLoop feeds incoming text to Process(), which buffers base64
// data between start/end markers and writes the decoded file on completion.
type CaptureState struct {
	mu          sync.Mutex
	collecting  bool
	buffer      strings.Builder
	startMarker string
	endMarker   string
	localPath   string
	done        chan struct{}
}

// Process examines incoming text from the client for capture markers.
// When the start marker is seen, buffering begins. When the end marker
// is seen, the buffered base64 is decoded and written to disk.
func (cs *CaptureState) Process(text string, s *Server, cid int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !cs.collecting {
			if strings.Contains(trimmed, cs.startMarker) {
				cs.collecting = true
				continue
			}
			continue
		}

		// Currently collecting
		if strings.Contains(trimmed, cs.endMarker) {
			// Done collecting -- decode and write
			cs.finalize(s, cid)
			return
		}

		// Skip certutil header/footer lines (BEGIN/END CERTIFICATE)
		if strings.HasPrefix(trimmed, "-----BEGIN ") || strings.HasPrefix(trimmed, "-----END ") {
			continue
		}

		cs.buffer.WriteString(trimmed)
	}
}

// finalize decodes the buffered base64 data and writes it to disk.
// Caller must hold cs.mu.
func (cs *CaptureState) finalize(s *Server, cid int) {
	raw := cs.buffer.String()
	// Strip any whitespace that crept in
	raw = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, raw)

	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Try RawStdEncoding (no padding)
		data, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			s.safePrintf("[!] download failed for client %d: base64 decode error: %v\n", cid, err)
			cs.cleanup()
			return
		}
	}

	if err := os.WriteFile(cs.localPath, data, 0o600); err != nil {
		s.safePrintf("[!] download failed for client %d: write error: %v\n", cid, err)
		cs.cleanup()
		return
	}

	abs, _ := filepath.Abs(cs.localPath)
	s.safePrintf("[*] download complete: client %d -> %s (%d bytes)\n", cid, abs, len(data))
	cs.cleanup()
}

// cleanup resets the capture state and signals completion.
func (cs *CaptureState) cleanup() {
	cs.collecting = false
	cs.buffer.Reset()
	if cs.done != nil {
		select {
		case <-cs.done:
		default:
			close(cs.done)
		}
	}
}

// IsDone returns true if the capture has completed.
func (cs *CaptureState) IsDone() bool {
	select {
	case <-cs.done:
		return true
	default:
		return false
	}
}

// randomHex returns n bytes of cryptographic randomness as a hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback: this should never happen
		return "000000"
	}
	return hex.EncodeToString(b)
}

// buildDownloadCommand builds the shell command to base64-encode a remote
// file, wrapped in unique start/end markers for capture.
func buildDownloadCommand(osType, remotePath, startMarker, endMarker string) string {
	switch osType {
	case "windows":
		// PowerShell: echo markers around base64 content
		return fmt.Sprintf(
			`powershell -Command "Write-Output '%s'; [Convert]::ToBase64String([IO.File]::ReadAllBytes('%s')); Write-Output '%s'"`,
			startMarker, remotePath, endMarker,
		)
	default: // linux
		return fmt.Sprintf(
			`echo '%s'; base64 '%s'; echo '%s'`,
			startMarker, remotePath, endMarker,
		)
	}
}

const (
	uploadMaxSize  = 10 * 1024 * 1024 // 10 MB
	uploadWarnSize = 1 * 1024 * 1024  // 1 MB
	chunkSize      = 4000             // max base64 chars per shell line
)

// buildUploadCommands builds the shell commands to decode base64 data
// and write it to a file on the remote client. Returns a slice of
// commands to send sequentially.
func buildUploadCommands(osType, remotePath, b64data string) []string {
	switch osType {
	case "windows":
		// For Windows, if data fits in one PowerShell command, send directly.
		// Otherwise chunk into a temp file and decode.
		if len(b64data) <= chunkSize {
			cmd := fmt.Sprintf(
				`powershell -Command "[IO.File]::WriteAllBytes('%s',[Convert]::FromBase64String('%s'))"`,
				remotePath, b64data,
			)
			return []string{cmd}
		}
		// Chunked approach: write base64 to temp file, then decode
		tag := randomHex(4)
		tmpFile := fmt.Sprintf(`C:\Windows\Temp\.ul_%s.b64`, tag)
		var cmds []string
		for i := 0; i < len(b64data); i += chunkSize {
			end := i + chunkSize
			if end > len(b64data) {
				end = len(b64data)
			}
			chunk := b64data[i:end]
			// Use cmd echo append (>> works on cmd and powershell)
			cmds = append(cmds, fmt.Sprintf(
				`powershell -Command "Add-Content -NoNewline -Path '%s' -Value '%s'"`,
				tmpFile, chunk,
			))
		}
		// Decode temp file and write to destination
		cmds = append(cmds, fmt.Sprintf(
			`powershell -Command "[IO.File]::WriteAllBytes('%s',[Convert]::FromBase64String((Get-Content -Raw '%s'))); Remove-Item '%s'"`,
			remotePath, tmpFile, tmpFile,
		))
		return cmds

	default: // linux
		if len(b64data) <= chunkSize {
			cmd := fmt.Sprintf(
				`echo '%s' | base64 -d > '%s'`,
				b64data, remotePath,
			)
			return []string{cmd}
		}
		// Chunked: write base64 to temp, then decode
		tag := randomHex(4)
		tmpFile := fmt.Sprintf("/tmp/.ul_%s.b64", tag)
		var cmds []string
		for i := 0; i < len(b64data); i += chunkSize {
			end := i + chunkSize
			if end > len(b64data) {
				end = len(b64data)
			}
			chunk := b64data[i:end]
			cmds = append(cmds, fmt.Sprintf(
				`printf '%%s' '%s' >> '%s'`,
				chunk, tmpFile,
			))
		}
		cmds = append(cmds, fmt.Sprintf(
			`base64 -d '%s' > '%s' && rm -f '%s'`,
			tmpFile, remotePath, tmpFile,
		))
		return cmds
	}
}
