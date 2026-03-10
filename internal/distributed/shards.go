package distributed

import "fmt"

type Shard struct {
	Index int `json:"index"`
	Start int `json:"start"`
	End   int `json:"end"`
}

func (s Shard) FrameCount() int {
	if s.End < s.Start {
		return 0
	}
	return s.End - s.Start + 1
}

// BuildShards splits [0,totalFrames-1] into contiguous shards.
func BuildShards(totalFrames, shardCount int) ([]Shard, error) {
	if totalFrames <= 0 {
		return nil, fmt.Errorf("totalFrames must be > 0")
	}
	if shardCount <= 0 {
		return nil, fmt.Errorf("shardCount must be > 0")
	}
	if shardCount > totalFrames {
		shardCount = totalFrames
	}

	base := totalFrames / shardCount
	extra := totalFrames % shardCount
	shards := make([]Shard, 0, shardCount)
	start := 0
	for i := 0; i < shardCount; i++ {
		size := base
		if i < extra {
			size++
		}
		end := start + size - 1
		shards = append(shards, Shard{
			Index: i,
			Start: start,
			End:   end,
		})
		start = end + 1
	}
	return shards, nil
}
