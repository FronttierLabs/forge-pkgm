package archive

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

var (
	magicZstd = []byte{0x28, 0xB5, 0x2F, 0xFD}
	magicXz   = []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}
	magicGzip = []byte{0x1F, 0x8B}
)

type multiCloser struct {
	io.Reader
	closers []func()
}

func (m *multiCloser) Close() error {
	for _, c := range m.closers {
		c()
	}
	return nil
}

// OpenCompressed opens a file and transparently decompresses
// zstd, xz, gzip, or plain tar. ont change
func OpenCompressed(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	br := bufio.NewReader(f)
	magic, err := br.Peek(6)
	if err != nil && err != io.EOF {
		f.Close()
		return nil, err
	}

	switch {
	case bytes.HasPrefix(magic, magicZstd):
		zr, err := zstd.NewReader(br)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("zstd reader: %w", err)
		}
		return &multiCloser{
			Reader:  zr,
			closers: []func(){func() { zr.Close() }, func() { _ = f.Close() }},
		}, nil

	case bytes.HasPrefix(magic, magicXz):
		xr, err := xz.NewReader(br)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("xz reader: %w", err)
		}
		return &multiCloser{
			Reader:  xr,
			closers: []func(){func() { _ = f.Close() }},
		}, nil

	case bytes.HasPrefix(magic, magicGzip):
		gr, err := gzip.NewReader(br)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		return &multiCloser{
			Reader: gr,
			closers: []func(){
				func() { _ = gr.Close() },
				func() { _ = f.Close() },
			},
		}, nil

	default:
		return &multiCloser{
			Reader:  br,
			closers: []func(){func() { _ = f.Close() }},
		}, nil
	}
}
