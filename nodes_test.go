package ctree

import (
	"io/fs"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tnode struct {
	name     string
	children []*tnode
	contents string
	mode     fs.FileMode
}

func (tn *tnode) build(t *testing.T, where string) {
	require := require.New(t)
	where = path.Join(where, tn.name)
	mode := tn.mode

	if mode == 0 {
		switch {
		case tn.contents != "":
			mode = 0666
		default:
			mode = 0777
		}
	}

	if len(tn.children) > 0 {
		err := os.Mkdir(where, mode)
		require.NoError(err)

		for _, tn := range tn.children {
			tn.build(t, where)
		}
		return
	}

	f, err := os.Create(where)
	defer f.Close()
	require.NoError(err)
	_, err = f.Write([]byte(tn.contents))
	require.NoError(err)
}

var ttree = &tnode{
	name: "home",
	children: []*tnode{
		{
			name: "ceswift",
			children: []*tnode{
				{
					name:     ".cshrc",
					contents: "echo hello COS",
				},
				{
					name: "bin",
					children: []*tnode{
						{
							name:     "worms",
							contents: "========8>",
						},
					},
				},
			},
		},
		{
			name: "wsfitzpa",
			children: []*tnode{
				{
					name:     ".cshrc",
					contents: "echo hello, william.",
				},
				{
					name: "bin",
					children: []*tnode{
						{
							name:     "zrun",
							contents: "uncompress $1 ; $1",
						},
					},
				},
			},
		},
	},
}

func getDNode(where string) (*DNode, error) {
	fi, err := os.Stat(where)
	if err != nil {
		return nil, err
	}
	node := newNode(where, &fi)
	return node.(*DNode), nil

}

func TestWork(t *testing.T) {
	where := t.TempDir()
	ttree.build(t, where)

	t.Run("call work directly", func(t *testing.T) {
		require := require.New(t)
		dn, err := getDNode(where)
		require.NoError(err)

		ws := make(workStream)
		ss := make(stopStream)
		var i int32
		dn.work(ws, ss, &i)
	})

	t.Run("Pure single-threaded", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		r := NewRoot(where)
		require.NotNil(r)
		r.Threads = 1
		r.WorkListSize = 0
		dn, err := r.Run()
		assert.NoError(err)
		require.NotNil(dn)
		assert.NoError(dn.Error())
	})

	t.Run("single threaded but queued", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		r := NewRoot(where)
		require.NotNil(r)
		r.Threads = 1
		dn, err := r.Run()
		assert.NoError(err)
		require.NotNil(dn)
		assert.NoError(dn.Error())
	})

	t.Run("multi threaded no queueing", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		r := NewRoot(where)
		require.NotNil(r)
		r.WorkListSize = 0
		dn, err := r.Run()
		assert.NoError(err)
		require.NotNil(dn)
		assert.NoError(dn.Error())
	})

	t.Run("happy path", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		r := NewRoot(where)
		require.NotNil(r)
		dn, err := r.Run()
		assert.NoError(err)
		require.NotNil(dn)
		assert.NoError(dn.Error())
	})

	t.Run("running on a file fails", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		where := path.Join(where, "home", "ceswift", ".cshrc")
		r := NewRoot(where)
		require.NotNil(r)
		dn, err := r.Run()
		assert.Error(err)
		assert.Nil(dn)
		assert.Contains(err.Error(), "not a directory")
	})

	t.Run("running on a non-existent fails", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		r := NewRoot("/does/not/exist")
		require.NotNil(r)
		dn, err := r.Run()
		assert.Nil(dn)
		require.Error(err)
		assert.Contains(err.Error(), "no such file or directory")
	})

	t.Run("Flatten() works", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		dn, err := NewRoot(where).Run()
		assert.NoError(err)
		require.NotNil(dn)
		assert.NoError(dn.Error())

		flat := dn.Flatten()
		assert.Equal(dn.TotalLength(), len(flat))
		visitator(assert, dn, flat)
	})

	t.Run("Errors() works", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		where := t.TempDir()
		ttree.build(t, where)

		bad := path.Join(where, "home", "wsfitzpa")
		err := os.Chmod(bad, 0)
		require.NoError(err)
		t.Cleanup(func() {
			os.Chmod(bad, 0777)
		})

		dn, err := NewRoot(where).Run()
		assert.NoError(err)
		require.NotNil(dn)

		errs := dn.Errors()
		require.NotEmpty(errs)
		assert.Equal(1, len(errs))
		assert.Contains(errs[0].Error(), bad)
		assert.Contains(errs[0].Error(), "permission")
	})

	t.Run("there are names", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		var visitor func(t *testing.T, dn *DNode) map[string]struct{}
		visitor = func(t *testing.T, dn *DNode) map[string]struct{} {
			names := map[string]struct{}{
				dn.name: {},
			}

			for _, leaf := range dn.leaves {
				names[leaf.name] = struct{}{}
			}

			for _, child := range dn.children {
				for name := range visitor(t, child) {
					names[name] = struct{}{}
				}
			}

			return names
		}

		r := NewRoot(where)
		dn, err := r.Run()
		assert.NoError(err)
		require.NotNil(dn)

		names := visitor(t, dn)
		assert.Contains(names, ".cshrc")
		assert.Contains(names, "wsfitzpa")
	})
}

func visitator(assert *assert.Assertions, dn *DNode, flat []Node) {
	assert.Contains(flat, dn)
	for _, leaf := range dn.leaves {
		assert.Contains(flat, leaf)
	}
	for _, child := range dn.children {
		visitator(assert, child, flat)
	}
}
