package runes

import (
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strconv"
)

// RuneBytes is the number of bytes in Go's rune type.
const runeBytes = 4

// A Buffer is an unbounded rune buffer backed by a file.
type Buffer struct {
	// F is the file that backs the buffer. It is created lazily.
	f ReaderWriterAt
	// BlockSize is the maximum number of runes in a block.
	blockSize int
	// Blocks contains all blocks of the buffer in order.
	// Free contains blocks that are free to be re-allocated.
	blocks, free []block
	// End is the byte offset of the end of the backing file.
	end int64

	// Cache is the index of the block whose data is currently cached.
	cached int
	// Cached0 is the address of the first rune in the cached block.
	cached0 int64
	// Cache is the cached data.
	cache []rune
	// Dirty tracks whether the cached data has changed since it was read.
	dirty bool

	// Size is the number of runes in the buffer.
	size int64
}

// A ReaderWriterAt implements the io.ReaderAt and io.WriterAt interfaces.
type ReaderWriterAt interface {
	io.ReaderAt
	io.WriterAt
}

// A block describes a portion of the buffer and its location in the backing file.
type block struct {
	// Start is the byte offset of the block in the file.
	start int64
	// N is the number of runes in the block.
	n int
}

// NewBuffer returns a new, empty buffer.
// No more than blockSize runes are cached in memory.
func NewBuffer(blockSize int) *Buffer {
	return &Buffer{
		blockSize: blockSize,
		cached:    -1,
		cache:     make([]rune, blockSize),
	}
}

// NewBufferReaderWriterAt is like NewBuffer but uses
// the given ReaderWriterAt as its backing store.
// If the ReaderWriterAt implements io.Closer, it is closed when the buffer is closed.
// If the ReaderWriterAt is an *os.File, the file is removed when the buffer is closed.
func NewBufferReaderWriterAt(blockSize int, f ReaderWriterAt) *Buffer {
	b := NewBuffer(blockSize)
	b.f = f
	return b
}

// Close closes the buffer and removes it's backing file.
func (b *Buffer) Close() error {
	b.cache = nil
	switch f := b.f.(type) {
	case *os.File:
		path := f.Name()
		if err := f.Close(); err != nil {
			return err
		}
		return os.Remove(path)
	case io.Closer:
		return f.Close()
	default:
		return nil
	}
}

// Size returns the number of runes in the buffer.
func (b *Buffer) Size() int64 { return b.size }

// Rune returns the rune at the given offset.
// If the rune is out of range it panics.
func (b *Buffer) Rune(offs int64) (rune, error) {
	if offs < 0 || offs > b.Size() {
		panic("rune index out of bounds")
	}
	if q0 := b.cached0; q0 <= offs && offs < q0+int64(b.blocks[b.cached].n) {
		return b.cache[offs-q0], nil
	}
	i, q0 := b.blockAt(offs)
	if _, err := b.get(i); err != nil {
		return -1, err
	}
	return b.cache[offs-q0], nil
}

// Read reads runes from the buffer beginning at a given offset.
// It is an error to read out of the range of the buffer.
func (b *Buffer) Read(n int, offs int64) ([]rune, error) {
	r := b.Reader(offs)
	r = &LimitedReader{Reader: r, N: int64(n)}
	return ReadAll(r)
}

type reader struct {
	*Buffer
	pos int64
}

func (r *reader) readSize() int64 { return r.Size() - r.pos }

// Reader returns a Reader that reads from the Buffer
// beginning at the given offset.
// The returned Reader need not be closed.
// The Buffer must not be modified
// between Read calls on the returned Reader.
func (b *Buffer) Reader(offs int64) Reader { return &reader{Buffer: b, pos: offs} }

func (r *reader) Read(p []rune) (int, error) {
	if r.pos < 0 || r.pos > r.Size() {
		return 0, os.ErrInvalid
	}
	if r.pos == r.Size() {
		return 0, io.EOF
	}
	i, blkStart := r.blockAt(r.pos)
	blk, err := r.get(i)
	if err != nil {
		return 0, err
	}
	cacheOffs := int(r.pos - blkStart)
	n := copy(p, r.cache[cacheOffs:blk.n])
	r.pos += int64(n)
	return n, nil
}

// Insert inserts runes into the buffer at the given offset.
// It is an error to insert at a point than is out of the range of the buffer.
func (b *Buffer) Insert(p []rune, offs int64) error {
	_, err := b.Writer(offs).Write(p)
	return err
}

// Writer returns a Writer that inserts into the Buffer
// beginning at the given offset.
// The returned Writer need not be closed.
func (b *Buffer) Writer(offs int64) Writer { return &writer{Buffer: b, pos: offs} }

type writer struct {
	*Buffer
	pos int64
}

func (w *writer) Write(p []rune) (int, error) {
	n, err := w.ReadFrom(&SliceReader{p})
	w.pos += n
	return int(n), err
}

func (w *writer) ReadFrom(r Reader) (int64, error) {
	return w.Buffer.ReaderFrom(w.pos).ReadFrom(r)
}

// ReaderFrom returns a ReaderFrom that inserts into the Buffer
// beginning at the given offset.
func (b *Buffer) ReaderFrom(offs int64) ReaderFrom {
	return &readerFrom{Buffer: b, pos: offs}
}

type readerFrom struct {
	*Buffer
	pos int64
}

func (dst *readerFrom) ReadFrom(r Reader) (int64, error) {
	if dst.pos < 0 || dst.pos > dst.Size() {
		return 0, os.ErrInvalid
	}
	if sz, ok := readSize(r); ok {
		return fastReadFrom(dst, r, sz)
	}
	return slowCopy(dst.Buffer.Writer(dst.pos), r)
}

func fastReadFrom(dst *readerFrom, r Reader, sz int64) (int64, error) {
	var tot int64
	for tot < sz {
		p, err := dst.makeSpace(sz-tot, dst.pos+tot)
		if err != nil {
			return tot, err
		}
		n, err := readFull(r, p)
		tot += int64(n)
		if err != nil {
			return tot, err
		}
	}
	// Sanity check: no more to read.
	if n, err := r.Read(make([]rune, 1)); n != 0 || err != io.EOF {
		panic("more to read")
	}
	return tot, nil
}

func readFull(r Reader, p []rune) (int, error) {
	var tot int
	for tot < len(p) {
		n, err := r.Read(p[tot:])
		tot += n
		if err != nil {
			if err == io.EOF {
				panic("not enough to read")
			}
			return tot, err
		}
	}
	return tot, nil
}

// ReadSize returns the exact number of runes to be read and true,
// or 0 and false if the exact number could not be determined.
func readSize(r Reader) (int64, bool) {
	switch r := r.(type) {
	case *LimitedReader:
		if sz, ok := readSize(r.Reader); ok {
			if r.N < sz {
				return r.N, true
			}
			return sz, true
		}

	case interface {
		readSize() int64
	}:
		return r.readSize(), true
	}
	return 0, false
}

// MakeSpace makes space in the buffer
// for up to n runes at the given address
// and returns a slice of the cache
// corresponding to the space.
func (b *Buffer) makeSpace(n, at int64) ([]rune, error) {
	i, blkStart := b.blockAt(at)
	blk, err := b.get(i)
	if err != nil {
		return nil, err
	}
	blkSpace := b.blockSize - blk.n
	if blkSpace == 0 {
		if i, err = b.insertAt(at); err != nil {
			return nil, err
		}
		if blk, err = b.get(i); err != nil {
			return nil, err
		}
		blkStart = at
		blkSpace = b.blockSize
	}
	if n < int64(blkSpace) {
		blkSpace = int(n)
	}
	cacheOffs := int(at - blkStart)
	copy(b.cache[cacheOffs+blkSpace:], b.cache[cacheOffs:blk.n])
	b.dirty = true
	blk.n += blkSpace
	b.size += int64(blkSpace)
	return b.cache[cacheOffs : cacheOffs+blkSpace], nil
}

// Delete deletes runes from the buffer starting at the given offset.
// It is an error to delete out of the range of the buffer.
func (b *Buffer) Delete(n, offs int64) error {
	if n < 0 {
		panic("bad count: " + strconv.FormatInt(n, 10))
	}
	if offs < 0 || offs+n > b.Size() {
		return errors.New("invalid offset: " + strconv.FormatInt(offs, 10))
	}
	for n > 0 {
		i, q0 := b.blockAt(offs)
		blk, err := b.get(i)
		if err != nil {
			return err
		}
		o := int(offs - q0)
		m := blk.n - o
		if int64(m) > n {
			m = int(n)
		}
		if o == 0 && n >= int64(blk.n) {
			// Remove the entire block.
			b.freeBlock(*blk)
			b.blocks = append(b.blocks[:i], b.blocks[i+1:]...)
			b.cached = -1
		} else {
			// Remove a portion of the block.
			copy(b.cache[o:], b.cache[o+m:])
			b.dirty = true
			blk.n -= m
		}
		n -= int64(m)
		b.size -= int64(m)
	}
	return nil
}

func (b *Buffer) allocBlock() block {
	if l := len(b.free); l > 0 {
		blk := b.free[l-1]
		b.free = b.free[:l-1]
		return blk
	}
	blk := block{start: b.end}
	b.end += int64(b.blockSize * runeBytes)
	return blk
}

func (b *Buffer) freeBlock(blk block) {
	b.free = append(b.free, block{start: blk.start})
}

// BlockAt returns the index and start address of the block containing the address.
// If the address is one beyond the end of the file, a new block is allocated.
// BlockAt panics if the address is negative or more than one past the end.
func (b *Buffer) blockAt(at int64) (int, int64) {
	if at < 0 || at > b.Size() {
		panic("invalid offset: " + strconv.FormatInt(at, 10))
	}
	if at == b.Size() {
		i := len(b.blocks)
		blk := b.allocBlock()
		b.blocks = append(b.blocks[:i], append([]block{blk}, b.blocks[i:]...)...)
		return i, at
	}
	var q0 int64
	for i, blk := range b.blocks {
		if q0 <= at && at < q0+int64(blk.n) {
			return i, q0
		}
		q0 += int64(blk.n)
	}
	panic("impossible")
}

// insertAt inserts a block at the address and returns the new block's index.
// If a block contains the address then it is split.
func (b *Buffer) insertAt(at int64) (int, error) {
	i, q0 := b.blockAt(at)
	o := int(at - q0)
	blk := b.blocks[i]
	if at == q0 {
		// Adding immediately before blk, no need to split.
		nblk := b.allocBlock()
		b.blocks = append(b.blocks[:i], append([]block{nblk}, b.blocks[i:]...)...)
		if b.cached == i {
			b.cached = i + 1
		}
		return i, nil
	}

	// Splitting blk.
	// Make sure it's both on disk and in the cache.
	if b.cached == i && b.dirty {
		if err := b.put(); err != nil {
			return -1, err
		}
	} else if _, err := b.get(i); err != nil {
		return -1, err
	}

	// Resize blk.
	b.blocks[i].n = int(o)

	// Insert the new, empty block.
	nblk := b.allocBlock()
	b.blocks = append(b.blocks[:i+1], append([]block{nblk}, b.blocks[i+1:]...)...)

	// Allocate a block for the second half of blk and set it as the cache.
	// The next put will write it out.
	nblk = b.allocBlock()
	b.blocks = append(b.blocks[:i+2], append([]block{nblk}, b.blocks[i+2:]...)...)
	b.blocks[i+2].n = blk.n - o
	copy(b.cache, b.cache[o:])
	b.cached = i + 2
	b.dirty = true

	return i + 1, nil
}

// File returns an *os.File, creating a new file if one is not created yet.
func (b *Buffer) file() (ReaderWriterAt, error) {
	if b.f == nil {
		f, err := ioutil.TempFile(os.TempDir(), "edit")
		if err != nil {
			return nil, err
		}
		b.f = f
	}
	return b.f, nil
}

// Put writes the cached block back to the file.
func (b *Buffer) put() error {
	if b.cached < 0 || !b.dirty || len(b.cache) == 0 {
		return nil
	}
	blk := b.blocks[b.cached]
	f, err := b.file()
	if err != nil {
		return err
	}
	bs := make([]byte, blk.n*runeBytes)
	for i, r := range b.cache[:blk.n] {
		binary.LittleEndian.PutUint32(bs[i*runeBytes:], uint32(r))
	}
	if _, err := f.WriteAt(bs, blk.start); err != nil {
		return err
	}
	b.dirty = false
	return nil
}

// Get loads the cache with the data from the block at the given index,
// returning a pointer to it.
func (b *Buffer) get(i int) (*block, error) {
	if b.cached == i {
		return &b.blocks[i], nil
	}
	if err := b.put(); err != nil {
		return nil, err
	}

	blk := b.blocks[i]
	f, err := b.file()
	if err != nil {
		return nil, err
	}
	bs := make([]byte, blk.n*runeBytes)
	if _, err := f.ReadAt(bs, blk.start); err != nil {
		if err == io.EOF {
			panic("unexpected EOF")
		}
		return nil, err
	}
	j := 0
	for len(bs) > 0 {
		b.cache[j] = rune(binary.LittleEndian.Uint32(bs))
		bs = bs[runeBytes:]
		j++
	}
	b.cached = i
	b.dirty = false
	b.cached0 = 0
	for j := 0; j < i; j++ {
		b.cached0 += int64(b.blocks[j].n)
	}
	return &b.blocks[i], nil
}
