package embedenc

import (
	"bytes"
	"errors"
	"github.com/ringsaturn/tzf/v2/internal/embedbin"
	"testing"

	pb "github.com/ringsaturn/tzf/v2/internal/model"
)

// TestTranscodeMMatchesEncodeM pins the transcoder to the reference encoder:
// converting any E file must reproduce EncodeM's output over the same source
// byte-for-byte, independent of E-side chunking and shortcut options.
func TestTranscodeMMatchesEncodeM(t *testing.T) {
	pre := &pb.PreindexTimezones{
		IdxZoom: 4, AggZoom: 2, Version: "edges",
		Keys: []*pb.PreindexTimezone{{Name: "A/West", X: 8, Y: 7, Z: 4}},
	}
	for name, opts := range map[string]EncodeOptions{
		"default":       {},
		"chunked":       {ChunkTarget: 2},
		"shortcut":      {AllowShortcut: true},
		"with-preindex": {Preindex: pre},
	} {
		t.Run(name, func(t *testing.T) {
			input := sharedEdgeFixture()
			_, r := openFixture(t, input, opts)
			got, err := r.TranscodeM()
			if err != nil {
				t.Fatal(err)
			}
			want, err := EncodeM(input, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("TranscodeM output differs from EncodeM")
			}
			if _, err := embedbin.Open(got); err != nil {
				t.Fatalf("transcoded file failed to open: %v", err)
			}
		})
	}
}

func TestTranscodeMRequiresEProfile(t *testing.T) {
	_, r := openMFixture(t, fixture("Etc/Test"), EncodeOptions{})
	if _, err := r.TranscodeM(); !errors.Is(err, embedbin.ErrProfile) {
		t.Fatalf("TranscodeM on M file = %v", err)
	}
}

func TestTranscodeMBundled(t *testing.T) {
	input := *loadDistTopo(t)
	_, r := openFixture(t, &input, EncodeOptions{AllowShortcut: true})
	got, err := r.TranscodeM()
	if err != nil {
		t.Fatal(err)
	}
	want, err := EncodeM(&input, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("bundled TranscodeM output differs from EncodeM")
	}
}
