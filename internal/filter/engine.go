package filter

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

func runFilter(r io.Reader, w io.Writer, backend backend, rules []Rule, maxLineBytes int) (Stats, error) {
	reader := bufio.NewReaderSize(r, 64*1024)
	writer := bufio.NewWriterSize(w, 64*1024)
	defer writer.Flush()

	var stats Stats
	insideSkippedBlock := false
	skipNextUnlock := false

	for {
		line, err := readLine(reader, maxLineBytes)
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, err
		}

		stats.TotalLines++
		trimmed := bytes.TrimSpace(line)

		if insideSkippedBlock {
			stats.FilteredLines++
			if bytes.HasSuffix(trimmed, []byte(";")) {
				insideSkippedBlock = false
			}
			continue
		}

		info, ok := backend.detectStatement(trimmed)
		if ok {
			if info.typ == statementUnlockTables && skipNextUnlock {
				stats.FilteredLines++
				skipNextUnlock = false
				continue
			}

			if shouldSkipStatement(info, rules) {
				stats.FilteredLines++
				if info.block && !bytes.HasSuffix(trimmed, []byte(";")) {
					insideSkippedBlock = true
				}
				if info.typ == statementLockTables {
					skipNextUnlock = true
				}
				continue
			}
		}

		if _, err := writer.Write(line); err != nil {
			return stats, fmt.Errorf("write output: %w", err)
		}
	}

	return stats, nil
}

func readLine(r *bufio.Reader, maxLineBytes int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLineBytes {
			return nil, fmt.Errorf("line exceeds MAX_LINE_BYTES=%d", maxLineBytes)
		}
		if err == nil {
			return buf, nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if len(buf) == 0 {
				return nil, io.EOF
			}
			return buf, nil
		}
		return nil, fmt.Errorf("read line: %w", err)
	}
}
