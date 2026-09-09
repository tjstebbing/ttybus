package protocol

import (
	"bufio"
	"io"
)

// ReadLine reads one protocol line from r, stripping a trailing \n and
// optional \r. Lines longer than MaxLine return ErrLineLen. A partial
// line at EOF is returned as a line (without error) if it has content;
// a clean EOF with no bytes returns io.EOF.
func ReadLine(r *bufio.Reader) (string, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > MaxLine {
			return "", ErrLineLen
		}
		if err == bufio.ErrBufferFull {
			buf = append(buf, chunk...)
			continue
		}
		if err == io.EOF {
			buf = append(buf, chunk...)
			if len(buf) == 0 {
				return "", io.EOF
			}
			return stripCR(string(buf)), nil
		}
		if err != nil {
			return "", err
		}
		// chunk includes the newline
		buf = append(buf, chunk...)
		buf = buf[:len(buf)-1] // drop \n
		return stripCR(string(buf)), nil
	}
}

func stripCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

// WriteLine writes s plus a newline. It does not flush.
func WriteLine(w io.Writer, s string) error {
	if len(s)+1 > MaxLine {
		return ErrLineLen
	}
	_, err := io.WriteString(w, s+"\n")
	return err
}
