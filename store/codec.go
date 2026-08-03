package store

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"

	"github.com/coxley/dg/document"
)

const maxDocumentSize = 64 << 20

func encodeDocument(doc document.Document) ([]byte, error) {
	data, err := document.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	if _, err := writer.Write(data); err != nil {
		return nil, errors.Join(fmt.Errorf("compress document: %w", err), writer.Close())
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish document compression: %w", err)
	}
	return encoded.Bytes(), nil
}

func decodeDocument(data []byte) (document.Document, error) {
	doc, _, err := decodeDocumentMigration(data)
	return doc, err
}

func decodeDocumentMigration(data []byte) (document.Document, bool, error) {
	var doc document.Document
	migrated, err := decodeDocumentMigrationInto(data, &doc)
	if err != nil {
		return document.Document{}, false, err
	}
	return doc, migrated, nil
}

func decodeDocumentInto(data []byte, dst *document.Document) error {
	_, err := decodeDocumentMigrationInto(data, dst)
	return err
}

func decodeDocumentMigrationInto(data []byte, dst *document.Document) (bool, error) {
	source := bytes.NewBuffer(data)
	reader, err := gzip.NewReader(source)
	if err != nil {
		return false, fmt.Errorf("open compressed document: %w", err)
	}
	reader.Multistream(false)
	plain, readErr := io.ReadAll(io.LimitReader(reader, maxDocumentSize+1))
	closeErr := reader.Close()
	if readErr != nil {
		return false, errors.Join(fmt.Errorf("decompress document: %w", readErr), closeErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close compressed document: %w", closeErr)
	}
	if len(plain) > maxDocumentSize {
		return false, fmt.Errorf("decompress document: exceeds %d-byte limit", maxDocumentSize)
	}
	if source.Len() != 0 {
		return false, errors.New("decode document: multiple gzip members")
	}
	return document.Migrate(plain, dst)
}
