package core

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxLogSize is the threshold at which log files are rotated (10MB).
const maxLogSize = 10 * 1024 * 1024

// Logger handles TSV-formatted forensic audit logging.
type Logger struct {
	mu        sync.Mutex
	sessionID string
	opsFile   *os.File
	delFile   *os.File
	opsPath   string
	delPath   string
}

// Operation records a high-level user command or system action.
type Operation struct {
	Timestamp string
	SessionID string
	Command   string
	Action    string
	Status    string
	Details   string
}

// Deletion records an individual file or directory removal.
type Deletion struct {
	Timestamp string
	SessionID string
	Path      string
	Bytes     int64
	Mode      string
	Command   string
}

// generateSessionID produces a unique UUID string for the logging session.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// rotateIfNeeded checks if the given file exceeds maxLogSize and rotates it.
func rotateIfNeeded(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if fi.Size() < maxLogSize {
		return nil
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	rotName := fmt.Sprintf("%s.%s%s", base, time.Now().UTC().Format("20060102-150405.000"), ext)
	return os.Rename(path, rotName)
}

// NewLogger creates a new Logger with a unique session ID.
// Log directory permissions are set to 0700 and log files to 0600.
func NewLogger() (*Logger, error) {
	if err := EnsureDirs(); err != nil {
		return nil, fmt.Errorf("failed to ensure log directories: %w", err)
	}

	logDir := LogDir()
	opsPath := filepath.Join(logDir, "operations.tsv")
	delPath := filepath.Join(logDir, "deletions.tsv")

	// Rotate before opening if already over 10MB
	if err := rotateIfNeeded(opsPath); err != nil {
		return nil, fmt.Errorf("failed to rotate operations log: %w", err)
	}
	if err := rotateIfNeeded(delPath); err != nil {
		return nil, fmt.Errorf("failed to rotate deletions log: %w", err)
	}

	opsFile, err := os.OpenFile(opsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open operations log: %w", err)
	}

	delFile, err := os.OpenFile(delPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		opsFile.Close()
		return nil, fmt.Errorf("failed to open deletions log: %w", err)
	}

	return &Logger{
		sessionID: generateSessionID(),
		opsFile:   opsFile,
		delFile:   delFile,
		opsPath:   opsPath,
		delPath:   delPath,
	}, nil
}

// sanitizeTSV replaces tabs and newlines to preserve TSV column integrity.
func sanitizeTSV(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// checkRotateOps rotates the operations log file if it exceeds maxLogSize.
func (l *Logger) checkRotateOps() {
	if l.opsFile == nil {
		return
	}

	fi, err := l.opsFile.Stat()
	if err != nil || fi.Size() < maxLogSize {
		return
	}

	_ = l.opsFile.Sync()
	_ = l.opsFile.Close()
	l.opsFile = nil

	_ = rotateIfNeeded(l.opsPath)

	f, err := os.OpenFile(l.opsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		l.opsFile = f
	}
}

// checkRotateDel rotates the deletions log file if it exceeds maxLogSize.
func (l *Logger) checkRotateDel() {
	if l.delFile == nil {
		return
	}

	fi, err := l.delFile.Stat()
	if err != nil || fi.Size() < maxLogSize {
		return
	}

	_ = l.delFile.Sync()
	_ = l.delFile.Close()
	l.delFile = nil

	_ = rotateIfNeeded(l.delPath)

	f, err := os.OpenFile(l.delPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		l.delFile = f
	}
}

// LogOperation records a high-level operation.
// Format: timestamp \t session_id \t command \t action \t status \t details
func (l *Logger) LogOperation(command, action, status, details string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.checkRotateOps()
	if l.opsFile == nil {
		return
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\n",
		timestamp,
		l.sessionID,
		sanitizeTSV(command),
		sanitizeTSV(action),
		sanitizeTSV(status),
		sanitizeTSV(details),
	)

	_, _ = l.opsFile.WriteString(line)
	_ = l.opsFile.Sync()
}

// LogDeletion records an individual file or directory removal.
// Format: timestamp \t session_id \t path \t bytes \t mode(trash|permanent) \t command
func (l *Logger) LogDeletion(path string, bytes int64, mode, command string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.checkRotateDel()
	if l.delFile == nil {
		return
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s\t%s\t%s\t%d\t%s\t%s\n",
		timestamp,
		l.sessionID,
		sanitizeTSV(path),
		bytes,
		sanitizeTSV(mode),
		sanitizeTSV(command),
	)

	_, _ = l.delFile.WriteString(line)
	_ = l.delFile.Sync()
}

// Close flushes and closes active log files.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.opsFile != nil {
		_ = l.opsFile.Sync()
		_ = l.opsFile.Close()
		l.opsFile = nil
	}

	if l.delFile != nil {
		_ = l.delFile.Sync()
		_ = l.delFile.Close()
		l.delFile = nil
	}
}

// ReadOperations reads recent operations from the operations log file.
// Returns the most recent operations (newest first). If limit <= 0, returns all operations.
func ReadOperations(limit int) ([]Operation, error) {
	opsPath := filepath.Join(LogDir(), "operations.tsv")
	file, err := os.Open(opsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open operations log: %w", err)
	}
	defer file.Close()

	var ops []Operation
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		ops = append(ops, Operation{
			Timestamp: parts[0],
			SessionID: parts[1],
			Command:   parts[2],
			Action:    parts[3],
			Status:    parts[4],
			Details:   parts[5],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read operations log: %w", err)
	}

	// Reverse to get most recent first
	n := len(ops)
	for i := 0; i < n/2; i++ {
		ops[i], ops[n-1-i] = ops[n-1-i], ops[i]
	}

	if limit > 0 && len(ops) > limit {
		ops = ops[:limit]
	}

	return ops, nil
}

// ReadDeletions reads recent deletions from the deletions log file.
// Returns the most recent deletions (newest first). If limit <= 0, returns all deletions.
func ReadDeletions(limit int) ([]Deletion, error) {
	delPath := filepath.Join(LogDir(), "deletions.tsv")
	file, err := os.Open(delPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open deletions log: %w", err)
	}
	defer file.Close()

	var dels []Deletion
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}

		b, _ := strconv.ParseInt(parts[3], 10, 64)
		dels = append(dels, Deletion{
			Timestamp: parts[0],
			SessionID: parts[1],
			Path:      parts[2],
			Bytes:     b,
			Mode:      parts[4],
			Command:   parts[5],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read deletions log: %w", err)
	}

	// Reverse to get most recent first
	n := len(dels)
	for i := 0; i < n/2; i++ {
		dels[i], dels[n-1-i] = dels[n-1-i], dels[i]
	}

	if limit > 0 && len(dels) > limit {
		dels = dels[:limit]
	}

	return dels, nil
}
