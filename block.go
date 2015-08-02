package honeybee

import (
	"sort"
	"sync"
	"time"
)

type Block struct {
	Origin Source
	// some unique id to identify this block
	Title       string
	ImageLink   string
	ImageWidth  int
	ImageHeight int
	Link        string
	Content     string
	TimeStamp   time.Time
	ModifyMtx   *sync.Mutex
}

func NewBlock(origin Source) *Block {
	return &Block{
		Origin:      origin,
		Title:       "",
		ImageLink:   "",
		Link:        "",
		Content:     "",
		TimeStamp:   time.Now(),
		ImageWidth:  0,
		ImageHeight: 0,
		ModifyMtx:   new(sync.Mutex),
	}
}

func (b *Block) Id() string {
	o_id := ""
	if b.Origin != nil {
		o_id = b.Origin.Id()
	}
	return IdEncodeStrings(o_id, b.Title, b.Link, b.Content)
}

func (b *Block) HasImage() bool {
	return b.ImageLink != ""
}

type ByTimeStamp []*Block

func (bt ByTimeStamp) Len() int {
	return len(bt)
}

func (bt ByTimeStamp) Swap(i, j int) {
	bt[i], bt[j] = bt[j], bt[i]
}

func (bt ByTimeStamp) Less(i, j int) bool {
	return bt[i].TimeStamp.After(bt[j].TimeStamp)
}

type BlockReceiver interface {
	ReceiveBlocks([]*Block)
}

type BlockProvider interface {
	GetBlocks() ([]*Block, error)
}

type BlockStore struct {
	blocks    []*Block
	index     map[string]*Block
	modifyMtx *sync.Mutex
}

func NewBlockStore() BlockStore {
	return BlockStore{
		blocks:    make([]*Block, 0),
		index:     make(map[string]*Block),
		modifyMtx: new(sync.Mutex),
	}
}

func (bs *BlockStore) List() []*Block {
	return bs.blocks
}

func (bs *BlockStore) Size() int {
	return len(bs.blocks)
}

func (bs *BlockStore) Get(blockId string) (b *Block, found bool) {
	b, found = bs.index[blockId]
	return
}

func (bs *BlockStore) ReceiveBlocks(newBlocks []*Block) {
	// collect the source ids of the blocks
	sourceIdSet := make(map[string]bool)
	for _, block := range newBlocks {
		if block.Origin != nil {
			sourceIdSet[block.Origin.Id()] = true
		}
	}

	bs.modifyMtx.Lock()
	defer bs.modifyMtx.Unlock()

	// purge blocks of the collected sources from current storage
	var blocksToKeep []*Block
	for _, block := range bs.blocks {
		_, found := sourceIdSet[block.Origin.Id()]
		if found {
			delete(bs.index, block.Id())
		} else {
			blocksToKeep = append(blocksToKeep, block)
		}
	}
	bs.blocks = blocksToKeep

	// append new ones
	for i := range newBlocks {
		bs.index[newBlocks[i].Id()] = newBlocks[i]
		bs.blocks = append(bs.blocks, newBlocks[i])
	}
	sort.Sort(ByTimeStamp(bs.blocks))
}
