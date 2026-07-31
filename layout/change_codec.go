package layout

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/coxley/dg/ir"
)

// MarshalJSON encodes snapshot's runtime state.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.state)
}

// UnmarshalJSON decodes and validates snapshot's runtime state.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	var state layoutHistoryState
	if err := decodeHistoryJSON(data, &state); err != nil {
		return fmt.Errorf("decode layout snapshot: %w", err)
	}
	if err := validateLayoutHistoryState(&state); err != nil {
		return fmt.Errorf("decode layout snapshot: %w", err)
	}
	s.state = state
	return nil
}

// MarshalJSON encodes change's runtime state.
func (c Change) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.value)
}

// UnmarshalJSON decodes and validates change's runtime state.
func (c *Change) UnmarshalJSON(data []byte) error {
	var change historyChange
	if err := decodeHistoryJSON(data, &change); err != nil {
		return fmt.Errorf("decode layout change: %w", err)
	}
	if err := validateHistoryChange(change); err != nil {
		return fmt.Errorf("decode layout change: %w", err)
	}
	c.value = change
	return nil
}

func decodeHistoryJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON value")
	}
	return nil
}

func validateLayoutHistoryState(state *layoutHistoryState) error {
	nodes := len(state.Graph.Nodes)
	edges := len(state.Graph.Edges)
	if len(state.Origins) != nodes ||
		len(state.Sizes) != nodes ||
		len(state.NodeStyles) != nodes ||
		len(state.Attachments) != nodes ||
		len(state.EdgeStyles) != edges ||
		len(state.EdgeBends) != edges {
		return errors.New("unaligned layout state")
	}
	if err := state.Graph.Validate(); err != nil {
		return fmt.Errorf("invalid graph: %w", err)
	}
	for portID, port := range state.Graph.Ports {
		if !state.Graph.PortExists(uint32(portID)) {
			continue
		}
		if !validHistorySide(port.Side) || port.Offset < 0 || port.Offset > 1 {
			return fmt.Errorf("port %d has invalid placement", portID)
		}
	}
	for nodeID, style := range state.NodeStyles {
		if !style.Valid() {
			return fmt.Errorf("node %d has invalid style", nodeID)
		}
	}
	for edgeID, style := range state.EdgeStyles {
		if !style.Valid() || !validPinnedBends(state.EdgeBends[edgeID]) {
			return fmt.Errorf("edge %d has invalid presentation", edgeID)
		}
	}
	if len(state.Order) != 0 {
		if err := validateDrawOrder(&state.Graph, state.Order); err != nil {
			return err
		}
	}
	for nodeID, attachment := range state.Attachments {
		if attachment == (Attachment{}) {
			continue
		}
		if attachment.NodeID != uint32(nodeID) ||
			attachment.Position == 0 ||
			attachment.Position == attachmentPositionMax ||
			!state.Graph.NodeExists(attachment.NodeID) ||
			!state.Graph.EdgeExists(attachment.EdgeID) {
			return errors.New("invalid attachment")
		}
		edge := state.Graph.Edges[attachment.EdgeID]
		if state.Graph.Ports[edge.PortA].Node == attachment.NodeID ||
			state.Graph.Ports[edge.PortB].Node == attachment.NodeID {
			return errors.New("attachment node is an edge endpoint")
		}
	}
	state.Graph = state.Graph.Clone()
	return nil
}

func validateHistoryChange(change historyChange) error {
	if change.Kind < historyCreateNode || change.Kind > historySetPinnedBends {
		return errors.New("invalid change kind")
	}
	if change.Kind == historySetLayer && !validHistoryHit(change.LayerHit) {
		return errors.New("invalid layer target")
	}
	if err := validateHistoryChangeState(change.Before); err != nil {
		return fmt.Errorf("invalid before state: %w", err)
	}
	if err := validateHistoryChangeState(change.After); err != nil {
		return fmt.Errorf("invalid after state: %w", err)
	}
	switch change.Kind {
	case historyCreateNode:
		if change.After.Node.ID != change.ID {
			return errors.New("created node ID does not match change ID")
		}
	case historyDeleteNode:
		if change.Before.Node.ID != change.ID {
			return errors.New("deleted node ID does not match change ID")
		}
	case historySetAttachment:
		if err := validateHistoryAttachmentChange(change); err != nil {
			return err
		}
	default:
	}
	return nil
}

func validateHistoryAttachmentChange(change historyChange) error {
	for _, state := range [...]historyChangeState{change.Before, change.After} {
		if !state.Attached {
			continue
		}
		if state.Attachment.NodeID != change.ID ||
			state.Attachment.Position == 0 ||
			state.Attachment.Position == attachmentPositionMax {
			return errors.New("invalid attachment change")
		}
	}
	return nil
}

func validateHistoryChangeState(state historyChangeState) error {
	if !state.NodeStyle.Valid() || !state.EdgeStyle.Valid() {
		return errors.New("invalid style")
	}
	if !validPinnedBends(state.Bends) {
		return errors.New("invalid bends")
	}
	if !state.Node.Style.Valid() {
		return errors.New("invalid node style")
	}
	for _, port := range state.Node.Ports {
		if !validHistorySide(port.Port.Side) || port.Port.Offset < 0 || port.Port.Offset > 1 {
			return errors.New("invalid node port placement")
		}
	}
	for _, edge := range state.Node.Edges {
		if !edge.Style.Valid() || !validPinnedBends(edge.Bends) {
			return errors.New("invalid node edge presentation")
		}
	}
	for _, layer := range state.Node.Layers {
		if !validHistoryHit(layer.Hit) {
			return errors.New("invalid node layer")
		}
	}
	return nil
}

func validHistoryHit(hit Hit) bool {
	return hit.Kind == HitNode || hit.Kind == HitEdge
}

func validHistorySide(side ir.Side) bool {
	return side == ir.Top || side == ir.RightSide || side == ir.Bottom || side == ir.LeftSide
}
