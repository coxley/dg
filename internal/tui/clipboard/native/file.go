package clipboard

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
)

const fileURIListMIME = "text/uri-list"

func encodeFileURI(path []byte) ([]byte, error) {
	if len(path) == 0 || bytes.IndexByte(path, 0) >= 0 {
		return nil, errors.New("invalid file path")
	}
	return []byte((&url.URL{Scheme: "file", Path: string(path)}).String()), nil
}

func encodeFileURIList(path []byte) ([]byte, error) {
	uri, err := encodeFileURI(path)
	if err != nil {
		return nil, err
	}
	return append(uri, '\r', '\n'), nil
}

func decodeFileURI(data []byte) ([]byte, error) {
	u, err := url.Parse(string(data))
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return nil, errors.New("invalid file URI")
	}
	if u.Path == "" || strings.IndexByte(u.Path, 0) >= 0 {
		return nil, errors.New("invalid file path")
	}
	return []byte(u.Path), nil
}

func decodeFileURIList(data []byte) ([]byte, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return decodeFileURI([]byte(line))
	}
	return nil, errors.New("file URI list contains no file")
}
