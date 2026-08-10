package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/coxley/dg/layout"
)

type attachmentV2 struct {
	Node     uint32 `json:"node"`
	Edge     uint32 `json:"edge"`
	Position uint16 `json:"position"`
	Anchor   Point  `json:"anchor"`
}

const attachmentBuildPassesV2 = 8

func encodedVersion(data []byte) (uint32, error) {
	var header struct {
		Version uint32 `json:"version"`
	}
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&header); err != nil {
		return 0, fmt.Errorf("decode document: %w", err)
	}
	return header.Version, nil
}

func migrateJSON(data []byte, version uint32) ([]byte, error) {
	for version != CurrentVersion {
		var err error
		switch version {
		case 2:
			data, err = migrateVersion2(data)
		case 3:
			data, err = migrateVersion3(data)
		default:
			return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, version)
		}
		if err != nil {
			return nil, fmt.Errorf("migrate document version %d: %w", version, err)
		}
		version++
	}
	return data, nil
}

func migrateVersion3(data []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode version 3 document: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode version 3 trailing data: %w", err)
	}
	fields["version"] = json.RawMessage("4")
	delete(fields, "groups")
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode version 4 fields: %w", err)
	}
	return data, nil
}

func migrateVersion2(data []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decode version 2 document: %w", err)
	}
	var attachments []attachmentV2
	if raw := fields["attachments"]; len(raw) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&attachments); err != nil {
			return nil, fmt.Errorf("decode attachments: %w", err)
		}
	}
	fields["version"] = json.RawMessage("3")
	if len(attachments) == 0 {
		delete(fields, "attachments")
	} else {
		fields["attachments"] = json.RawMessage("[]")
	}
	current, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode version 3 fields: %w", err)
	}
	if len(attachments) == 0 {
		return current, nil
	}
	var doc Document
	if err := decodeJSONInto(current, &doc); err != nil {
		return nil, err
	}
	doc.Version = CurrentVersion
	geo, err := doc.Convert()
	if err != nil {
		return nil, err
	}
	type target struct {
		attachment attachmentV2
		point      layout.Point
	}
	targets := make([]target, len(attachments))
	hosts := make([]layout.AttachmentHost, 0, len(attachments))
	seen := make([]uint32, 0, len(attachments))
	for i, attachment := range attachments {
		if uint64(attachment.Node) >= uint64(len(doc.Nodes)) {
			return nil, fmt.Errorf("attachment %d references unknown node %d", i, attachment.Node)
		}
		if uint64(attachment.Edge) >= uint64(len(doc.Edges)) {
			return nil, fmt.Errorf("attachment %d references unknown edge %d", i, attachment.Edge)
		}
		if slices.Contains(seen, attachment.Node) {
			return nil, fmt.Errorf("attachment %d duplicates node %d", i, attachment.Node)
		}
		seen = append(seen, attachment.Node)
		if attachment.Position == 0 || attachment.Position == math.MaxUint16 {
			return nil, fmt.Errorf("attachment %d overlaps an edge endpoint", i)
		}
		targets[i].attachment = attachment
		hosts = append(hosts, layout.AttachmentHost{
			NodeID: attachment.Node,
			EdgeID: attachment.Edge,
		})
	}
	stable := false
	for range attachmentBuildPassesV2 {
		if err := geo.RouteAttachmentHosts(hosts...); err != nil {
			return nil, err
		}
		changed := false
		for i := range targets {
			target := &targets[i]
			point, err := attachmentPointV2(
				geo.Edges[target.attachment.Edge].Points,
				target.attachment.Position,
			)
			if err != nil {
				return nil, fmt.Errorf("attachment %d: %w", i, err)
			}
			if point.X < target.attachment.Anchor.X || point.Y < target.attachment.Anchor.Y {
				return nil, fmt.Errorf("attachment %d exceeds coordinate space", i)
			}
			target.point = point
			origin := layout.NewPoint(
				point.X-target.attachment.Anchor.X,
				point.Y-target.attachment.Anchor.Y,
			)
			if geo.Nodes[target.attachment.Node].Rect.Min == origin {
				continue
			}
			if err := geo.PlaceNode(target.attachment.Node, origin); err != nil {
				return nil, fmt.Errorf("place attachment %d: %w", i, err)
			}
			changed = true
		}
		if !changed {
			stable = true
			break
		}
	}
	if !stable {
		return nil, errors.New("attachment routing did not converge")
	}
	converted := make([]layout.Attachment, 0, len(targets))
	for i, target := range targets {
		attachment, err := geo.AttachmentAt(
			target.attachment.Node,
			target.attachment.Edge,
			target.point,
		)
		if err != nil {
			return nil, fmt.Errorf("convert attachment %d: %w", i, err)
		}
		converted = append(converted, attachment)
	}
	if err := geo.SetAttachments(converted...); err != nil {
		return nil, fmt.Errorf("restore attachments: %w", err)
	}
	doc.Update(geo)
	doc.Version = 3
	doc.Groups = nil
	return Marshal(doc)
}

func attachmentPointV2(points []layout.Point, position uint16) (layout.Point, error) {
	var total uint64
	for i := 1; i < len(points); i++ {
		total += manhattan(points[i-1], points[i])
	}
	if total == 0 {
		return layout.Point{}, errors.New("empty attachment route")
	}
	distance := (uint64(position)*total + math.MaxUint16/2) / math.MaxUint16
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		segment := manhattan(a, b)
		if distance > segment {
			distance -= segment
			continue
		}
		delta := uint32(distance)
		switch {
		case a.X == b.X && b.Y >= a.Y:
			return layout.NewPoint(a.X, a.Y+delta), nil
		case a.X == b.X:
			return layout.NewPoint(a.X, a.Y-delta), nil
		case b.X >= a.X:
			return layout.NewPoint(a.X+delta, a.Y), nil
		default:
			return layout.NewPoint(a.X-delta, a.Y), nil
		}
	}
	return points[len(points)-1], nil
}

func manhattan(a, b layout.Point) uint64 {
	return uint64(max(a.X, b.X)-min(a.X, b.X)) +
		uint64(max(a.Y, b.Y)-min(a.Y, b.Y))
}
