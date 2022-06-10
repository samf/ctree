package ctree

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

const (
	// DefaultThreads is how many threads are assigned by default
	DefaultThreads = 4
	// DefaultWorkListSize is how many directory nodes can be enqueued for
	// worker threads to process
	DefaultWorkListSize = 1024
)

type workStream chan *DNode
type stopStream chan struct{}

// Root is the root of a directory tree to be walked
type Root struct {
	Path         string
	Threads      int
	WorkListSize int

	work    workStream
	stop    stopStream
	pending int32
	wg      sync.WaitGroup
}

// NewRoot creates a Root node
func NewRoot(path string) *Root {
	return &Root{
		Path:         path,
		Threads:      DefaultThreads,
		WorkListSize: DefaultWorkListSize,
	}
}

// Run walks the directory tree at the Root, returning a DNode
func (r *Root) Run() (*DNode, error) {
	r.setup()

	fi, err := os.Stat(r.Path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%q: not a directory", r.Path)
	}
	dn := newNode(r.Path, &fi).(*DNode)
	if err != nil {
		return nil, err
	}
	go func() {
		r.work <- dn
	}()

	for i := 0; i < r.Threads; i++ {
		r.wg.Add(1)
		r.allWork()
	}

	r.wg.Wait()

	return dn, nil
}

func (r *Root) allWork() {
	var dn *DNode

	defer r.wg.Done()

	for {
		select {
		case <-r.stop:
			return
		case dn = <-r.work:
			dn.work(r.work, r.stop, &r.pending)
			if atomic.AddInt32(&r.pending, -1) < 1 {
				close(r.stop)
				return
			}
		}
	}
}

func (r *Root) setup() {
	if r.Threads <= 0 {
		r.Threads = DefaultThreads
	}

	if r.WorkListSize < 0 {
		r.WorkListSize = DefaultWorkListSize
	}

	r.work = make(workStream, r.WorkListSize)
	r.stop = make(stopStream)
	r.pending = 1
}
