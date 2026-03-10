package distributed

import "testing"

func TestBuildShards(t *testing.T) {
	shards, err := BuildShards(10, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("expected 3 shards, got %d", len(shards))
	}
	if shards[0].Start != 0 || shards[0].End != 3 {
		t.Fatalf("unexpected shard 0: %+v", shards[0])
	}
	if shards[1].Start != 4 || shards[1].End != 6 {
		t.Fatalf("unexpected shard 1: %+v", shards[1])
	}
	if shards[2].Start != 7 || shards[2].End != 9 {
		t.Fatalf("unexpected shard 2: %+v", shards[2])
	}
}

func TestBuildShardsValidation(t *testing.T) {
	if _, err := BuildShards(0, 2); err == nil {
		t.Fatalf("expected error for totalFrames <= 0")
	}
	if _, err := BuildShards(10, 0); err == nil {
		t.Fatalf("expected error for shardCount <= 0")
	}
}

func TestBuildShardsClampsShardCount(t *testing.T) {
	shards, err := BuildShards(3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("expected 3 shards, got %d", len(shards))
	}
	for i, s := range shards {
		if s.Start != i || s.End != i {
			t.Fatalf("unexpected shard %d: %+v", i, s)
		}
	}
}
