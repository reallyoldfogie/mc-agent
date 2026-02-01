package pathfinding

import (
	"container/heap"
	"fmt"
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// AbstractNode represents a node in the abstract graph (an entrance point)
type AbstractNode struct {
	// Entrance is the entrance this node represents (nil for temporary nodes)
	Entrance *Entrance
	// Edges are the outgoing edges from this node
	Edges []*AbstractEdge
	// Temporary nodes are used for inserting start/goal positions
	IsTemporary bool
	// Position is used for temporary nodes
	TempPosition models.V3
}

// String returns a string representation of the abstract node
func (n *AbstractNode) String() string {
	if n.IsTemporary {
		return fmt.Sprintf("TempNode(%.0f,%.0f,%.0f)", n.TempPosition.X, n.TempPosition.Y, n.TempPosition.Z)
	}
	if n.Entrance != nil {
		return fmt.Sprintf("Node[%s]", n.Entrance.String())
	}
	return "Node[nil]"
}

// GetPosition returns the position of this node
func (n *AbstractNode) GetPosition() models.V3 {
	if n.IsTemporary {
		return n.TempPosition
	}
	if n.Entrance != nil {
		return n.Entrance.Pos1
	}
	return models.V3{}
}

// AbstractEdge represents a connection between two abstract nodes
type AbstractEdge struct {
	// To is the destination node
	To *AbstractNode
	// Cost is the total movement cost of the path
	Cost float64
	// Path is the cached low-level path (may be nil for edges being built)
	Path *Path
}

// String returns a string representation of the abstract edge
func (e *AbstractEdge) String() string {
	return fmt.Sprintf("Edge[->%s cost=%.2f]", e.To.String(), e.Cost)
}

// AbstractGraph represents the high-level graph of cluster entrances
type AbstractGraph struct {
	// Nodes maps entrances to their abstract nodes
	Nodes map[*Entrance]*AbstractNode
	// ClusterSize is the size of each cluster dimension
	ClusterSize int
}

// NewAbstractGraph creates a new abstract graph
func NewAbstractGraph(clusterSize int) *AbstractGraph {
	return &AbstractGraph{
		Nodes:       make(map[*Entrance]*AbstractNode),
		ClusterSize: clusterSize,
	}
}

// GetOrCreateNode gets or creates an abstract node for an entrance
func (g *AbstractGraph) GetOrCreateNode(entrance *Entrance) *AbstractNode {
	if node, exists := g.Nodes[entrance]; exists {
		return node
	}
	node := &AbstractNode{
		Entrance:    entrance,
		Edges:       make([]*AbstractEdge, 0),
		IsTemporary: false,
	}
	g.Nodes[entrance] = node
	return node
}

// AddEdge adds an edge from one entrance to another
func (g *AbstractGraph) AddEdge(from, to *Entrance, cost float64, path *Path) {
	fromNode := g.GetOrCreateNode(from)
	toNode := g.GetOrCreateNode(to)

	edge := &AbstractEdge{
		To:   toNode,
		Cost: cost,
		Path: path,
	}
	fromNode.Edges = append(fromNode.Edges, edge)
}

// RemoveNode removes a node from the graph (used for temporary nodes)
func (g *AbstractGraph) RemoveNode(node *AbstractNode) {
	if node.Entrance != nil {
		delete(g.Nodes, node.Entrance)
	}

	// Remove edges pointing to this node
	for _, n := range g.Nodes {
		newEdges := make([]*AbstractEdge, 0)
		for _, edge := range n.Edges {
			if edge.To != node {
				newEdges = append(newEdges, edge)
			}
		}
		n.Edges = newEdges
	}
}

// Clear removes all nodes and edges from the graph
func (g *AbstractGraph) Clear() {
	g.Nodes = make(map[*Entrance]*AbstractNode)
}

// abstractSearchNode represents a node in the A* search on the abstract graph
type abstractSearchNode struct {
	node     *AbstractNode
	parent   *abstractSearchNode
	gCost    float64
	hCost    float64
	fCost    float64
	index    int
	fromEdge *AbstractEdge // The edge used to reach this node
}

// abstractNodeHeap implements heap.Interface for abstract A* search
type abstractNodeHeap []*abstractSearchNode

func (h abstractNodeHeap) Len() int           { return len(h) }
func (h abstractNodeHeap) Less(i, j int) bool { return h[i].fCost < h[j].fCost }
func (h abstractNodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *abstractNodeHeap) Push(x any) {
	n := len(*h)
	item := x.(*abstractSearchNode)
	item.index = n
	*h = append(*h, item)
}

func (h *abstractNodeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// SearchAbstractGraph performs A* search on the abstract graph
func (g *AbstractGraph) SearchAbstractGraph(startNodes, goalNodes []*AbstractNode) []*AbstractEdge {
	if len(startNodes) == 0 || len(goalNodes) == 0 {
		return nil
	}

	// Create goal set for quick lookup
	goalSet := make(map[*AbstractNode]bool)
	for _, node := range goalNodes {
		goalSet[node] = true
	}

	// Initialize A* search
	openSet := &abstractNodeHeap{}
	heap.Init(openSet)

	closedSet := make(map[*AbstractNode]bool)
	gScores := make(map[*AbstractNode]float64)

	// Add all start nodes to open set
	for _, startNode := range startNodes {
		// Use first goal node for heuristic
		goalPos := goalNodes[0].GetPosition()
		startPos := startNode.GetPosition()

		searchNode := &abstractSearchNode{
			node:   startNode,
			parent: nil,
			gCost:  0,
			hCost:  abstractHeuristic(startPos, goalPos),
		}
		searchNode.fCost = searchNode.gCost + searchNode.hCost
		heap.Push(openSet, searchNode)
		gScores[startNode] = 0
	}

	nodesExpanded := 0

	// A* main loop
	for openSet.Len() > 0 {
		current := heap.Pop(openSet).(*abstractSearchNode)

		// Check if we reached any goal
		if goalSet[current.node] {
			// Reconstruct path (as list of edges)
			return reconstructAbstractPath(current)
		}

		closedSet[current.node] = true

		// Explore neighbors
		nodesExpanded++
		for _, edge := range current.node.Edges {
			neighbor := edge.To

			if closedSet[neighbor] {
				continue
			}

			tentativeGCost := current.gCost + edge.Cost

			existingGCost, exists := gScores[neighbor]
			if !exists || tentativeGCost < existingGCost {
				gScores[neighbor] = tentativeGCost

				goalPos := goalNodes[0].GetPosition()
				neighborPos := neighbor.GetPosition()

				searchNode := &abstractSearchNode{
					node:     neighbor,
					parent:   current,
					gCost:    tentativeGCost,
					hCost:    abstractHeuristic(neighborPos, goalPos),
					fromEdge: edge,
				}
				searchNode.fCost = searchNode.gCost + searchNode.hCost
				heap.Push(openSet, searchNode)
			}
		}
	}

	// No path found
	log.Printf("[HPA* Graph] Abstract search failed: expanded %d nodes, no path found", nodesExpanded)
	return nil
}

// reconstructAbstractPath reconstructs the path from the goal node
func reconstructAbstractPath(goalNode *abstractSearchNode) []*AbstractEdge {
	path := make([]*AbstractEdge, 0)
	current := goalNode

	for current.parent != nil {
		if current.fromEdge != nil {
			path = append(path, current.fromEdge)
		}
		current = current.parent
	}

	// Reverse to get start -> goal order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// abstractHeuristic calculates the heuristic cost for abstract graph search
// Uses Euclidean distance as straight-line distance between cluster entrances
func abstractHeuristic(pos, goal models.V3) float64 {
	dx := goal.X - pos.X
	dy := goal.Y - pos.Y
	dz := goal.Z - pos.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
