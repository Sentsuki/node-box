package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"node-box/upstream/model/singbox"
)

func benchmarkTemplateJSON(nodeCount int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"metadata":{"keep":true},"outbounds":[`)
	for i := range nodeCount {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"type":"urltest","tag":"template-%d","outbounds":["{all}"]}`, i)
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

func benchmarkSingOuts(nodeCount int) []singbox.SingBoxOut {
	out := make([]singbox.SingBoxOut, nodeCount)
	for i := range out {
		out[i] = singbox.SingBoxOut{Type: "vmess", Tag: fmt.Sprintf("node-%d", i)}
	}
	return out
}

func BenchmarkPatchMap(b *testing.B) {
	tpl := benchmarkTemplateJSON(100)
	s := benchmarkSingOuts(100)
	b.ReportAllocs()
	b.SetBytes(int64(len(tpl)))

	for b.Loop() {
		_, err := PatchMap(tpl, s, nil, "", "", nil, nil, false, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPatchMapFromMap(b *testing.B) {
	tpl := benchmarkTemplateJSON(100)
	var decoded map[string]any
	if err := json.Unmarshal(tpl, &decoded); err != nil {
		b.Fatal(err)
	}
	s := benchmarkSingOuts(100)
	b.ReportAllocs()
	b.SetBytes(int64(len(tpl)))

	for b.Loop() {
		_, err := PatchMapFromMap(decoded, s, nil, "", "", nil, nil, false, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}
