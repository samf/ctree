package ctree

import (
	"os"
	"path"
	"sync/atomic"
)

// DNode describes a directory, potentially an interior node on the graph
type DNode struct {
	name     string
	path     string
	parent   *DNode
	info     *os.FileInfo
	children []*DNode
	leaves   []*Leaf
	err      error
}

var _ Node = &DNode{}

// Path returns the path of the directory node
func (dn *DNode) Path() string {
	return dn.path
}

// Info returns the FileInfo of the directory node
func (dn *DNode) Info() *os.FileInfo {
	return dn.info
}

// Error returns any error that may have occurred when processing this node
func (dn *DNode) Error() error {
	return dn.err
}

// TotalLength counts the number of nodes
func (dn *DNode) TotalLength() int {
	l := len(dn.leaves) + 1 // +1 to count yourself

	for _, child := range dn.children {
		l += child.TotalLength()
	}

	return l
}

// Flatten flattens the dnode tree into a slice of nodes
func (dn *DNode) Flatten() []Node {
	nodes := make(
		[]Node,
		1+len(dn.leaves),
		1+len(dn.leaves)+len(dn.children),
	)

	nodes[0] = dn

	for i := range dn.leaves {
		nodes[i+1] = dn.leaves[i]
	}

	for _, child := range dn.children {
		nodes = append(nodes, child.Flatten()...)
	}

	return nodes
}

// Errors returns a slice of all of the errors contained in the DNode
func (dn *DNode) Errors() []error {
	errs := []error{}

	if dn.err != nil {
		errs = append(errs, dn.err)
	}

	for _, child := range dn.children {
		errs = append(errs, child.Errors()...)
	}

	return errs
}

// Leaf holds information on a leaf node
type Leaf struct {
	name   string
	path   string
	parent *DNode
	info   *os.FileInfo
}

var _ Node = &Leaf{}

// Path returns the path of the leaf node
func (l *Leaf) Path() string {
	return l.path
}

// Info returns the FileInfo of the leaf node
func (l *Leaf) Info() *os.FileInfo {
	return l.info
}

// Node is an interface for nodes on the graph
type Node interface {
	Path() string
	Info() *os.FileInfo
}

func newNode(fullpath string, fi *os.FileInfo) Node {
	name := path.Base(fullpath)
	if (*fi).IsDir() {
		return &DNode{
			path: fullpath,
			name: name,
			info: fi,
		}
	}

	return &Leaf{
		path: fullpath,
		name: name,
		info: fi,
	}
}

func (dn *DNode) work(work workStream, stop stopStream, pending *int32) {
	f, err := os.Open(dn.path)
	if err != nil {
		dn.err = err
		return
	}

	infos, err := f.Readdir(0)
	if err != nil {
		dn.err = err
		f.Close()
		return
	}
	f.Close()

	for _, fi := range infos {
		switch node := newNode(path.Join(dn.path, fi.Name()), &fi).(type) {
		case *DNode:
			node.parent = dn
			dn.children = append(dn.children, node)
		case *Leaf:
			node.parent = dn
			dn.leaves = append(dn.leaves, node)
		}
	}

	for _, dn := range dn.children {
		select {
		case <-stop:
			return
		case work <- dn:
			atomic.AddInt32(pending, 1)
		default:
			dn.work(work, stop, pending)
		}
	}
}
