package ir

import "testing"

func BenchmarkComponentsBuild(b *testing.B) {
	const nodeCount = 1024

	var graph Graph
	for range nodeCount {
		graph.NewNode("node")
	}
	for nodeID := uint32(1); nodeID < nodeCount; nodeID++ {
		graph.ConnectNodes(nodeID-1, RightSide, LeftSide, nodeID)
	}
	for nodeID := uint32(16); nodeID < nodeCount; nodeID++ {
		graph.ConnectNodes(nodeID-16, Bottom, Top, nodeID)
	}

	var components Components
	components.Build(&graph)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		components.Build(&graph)
	}
}
