package sync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

func TestGzipMediaType(t *testing.T) {
	if got := gzipMediaType(types.OCILayerZStd); got != types.OCILayer {
		t.Fatalf("oci = %s", got)
	}
	nondistributable := types.MediaType("application/vnd.oci.image.layer.nondistributable.v1.tar+zstd")
	if got := gzipMediaType(nondistributable); got != types.OCIRestrictedLayer {
		t.Fatalf("nondistributable = %s", got)
	}
	dockerZstd := types.MediaType("application/vnd.docker.image.rootfs.diff.tar+zstd")
	if got := gzipMediaType(dockerZstd); got != types.DockerLayer {
		t.Fatalf("docker = %s", got)
	}
}

func TestACRPersonalHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{host: "registry.cn-hangzhou.aliyuncs.com", want: true},
		{host: "registry.ap-southeast-1.aliyuncs.com", want: true},
		{host: "Registry.CN-Hangzhou.Aliyuncs.COM:443", want: true},
		{host: "ytoken-registry.cn-hangzhou.cr.aliyuncs.com", want: false},
		{host: "docker.io", want: false},
	}
	for _, tc := range cases {
		if got := acrPersonalHost(tc.host); got != tc.want {
			t.Errorf("acrPersonalHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestPrepareForRegistryRecompressesZstdLayers(t *testing.T) {
	tarContent := tarBytes(t, "app.txt", "rsshub")
	img := imageWithLayers(t, zstdLayer(t, tarContent), types.OCILayerZStd)
	beforeConfig := rawConfig(t, img)
	originalDigest := layerDigest(t, img, 0)

	ref := mustTag(t, "registry.cn-hangzhou.aliyuncs.com/ytoken/docker.io.diygod.rsshub:latest")
	got, err := prepareForRegistry(ref, img)
	if err != nil {
		t.Fatal(err)
	}
	gotImg, ok := got.(v1.Image)
	if !ok {
		t.Fatalf("got %T", got)
	}
	if !bytes.Equal(rawConfig(t, gotImg), beforeConfig) {
		t.Fatal("镜像 config 被改写")
	}
	manifest := imageManifest(t, gotImg)
	if len(manifest.Layers) != 1 {
		t.Fatalf("层数 = %d", len(manifest.Layers))
	}
	layer := manifest.Layers[0]
	if layer.MediaType != types.OCILayer {
		t.Fatalf("mediaType = %s", layer.MediaType)
	}
	if layer.Digest == originalDigest {
		t.Fatal("zstd 层摘要未改变")
	}
	if layer.Annotations["org.opencontainers.image.title"] != "app" {
		t.Fatalf("annotations = %#v", layer.Annotations)
	}
	gotLayer, err := gotImg.LayerByDigest(layer.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if size, err := gotLayer.Size(); err != nil || size != layer.Size {
		t.Fatalf("size = %d, %v; manifest = %d", size, err, layer.Size)
	}
	body := readAll(t, mustCompressed(t, gotLayer))
	if !bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		t.Fatalf("gzip magic = %x", body[:min(2, len(body))])
	}
	if layer.Size != int64(len(body)) {
		t.Fatalf("manifest size = %d, body = %d", layer.Size, len(body))
	}
	sum, _, err := v1.SHA256(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if sum != layer.Digest {
		t.Fatalf("digest = %s, body = %s", layer.Digest, sum)
	}
	unzipped := gunzip(t, body)
	if !bytes.Equal(unzipped, tarContent) {
		t.Fatal("gzip 解压后的 tar 与源层不一致")
	}
	diffID, err := gotLayer.DiffID()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gotImg.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RootFS.DiffIDs) != 1 || cfg.RootFS.DiffIDs[0] != diffID {
		t.Fatalf("diffID = %s, config = %#v", diffID, cfg.RootFS.DiffIDs)
	}
	raw, err := gotImg.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "+zstd") {
		t.Fatalf("清单仍包含 zstd: %s", raw)
	}
}

func TestPrepareForRegistryRecompressesIndexChildren(t *testing.T) {
	amd := imageWithLayers(t, zstdLayer(t, tarBytes(t, "amd.txt", "amd64")), types.OCILayerZStd)
	arm := imageWithLayers(t, zstdLayer(t, tarBytes(t, "arm.txt", "arm64")), types.OCILayerZStd)
	amdDigest, err := amd.Digest()
	if err != nil {
		t.Fatal(err)
	}
	armDigest, err := arm.Digest()
	if err != nil {
		t.Fatal(err)
	}
	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add: amd,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{OS: "linux", Architecture: "amd64"},
			},
		},
		mutate.IndexAddendum{
			Add: arm,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{OS: "linux", Architecture: "arm64"},
			},
		},
	)
	ref := mustTag(t, "registry.cn-hongkong.aliyuncs.com/ytoken/rsshub:latest")
	got, err := prepareForRegistry(ref, idx)
	if err != nil {
		t.Fatal(err)
	}
	gotIdx, ok := got.(v1.ImageIndex)
	if !ok {
		t.Fatalf("got %T", got)
	}
	manifest, err := gotIdx.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.MediaType != types.OCIImageIndex {
		t.Fatalf("index mediaType = %s", manifest.MediaType)
	}
	if len(manifest.Manifests) != 2 {
		t.Fatalf("清单数 = %d", len(manifest.Manifests))
	}
	seen := map[string]bool{}
	for _, desc := range manifest.Manifests {
		if desc.Digest == amdDigest || desc.Digest == armDigest {
			t.Fatalf("子清单摘要未更新: %s", desc.Digest)
		}
		if !desc.MediaType.IsImage() {
			t.Fatalf("子清单类型 = %s", desc.MediaType)
		}
		if desc.Platform == nil {
			t.Fatal("缺少平台")
		}
		seen[desc.Platform.Architecture] = true
		child, err := gotIdx.Image(desc.Digest)
		if err != nil {
			t.Fatal(err)
		}
		layers := imageManifest(t, child).Layers
		if len(layers) != 1 || layers[0].MediaType != types.OCILayer {
			t.Fatalf("子层 = %+v", layers)
		}
	}
	if !seen["amd64"] || !seen["arm64"] {
		t.Fatalf("平台 = %#v", seen)
	}
	children, err := partial.Manifests(gotIdx)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("可推送的子清单 = %d", len(children))
	}
	for _, child := range children {
		img, ok := child.(v1.Image)
		if !ok {
			t.Fatalf("子清单 = %T", child)
		}
		if _, err := img.Layers(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrepareForRegistryKeepsZstdOutsideACRPersonal(t *testing.T) {
	img := imageWithLayers(t, zstdLayer(t, tarBytes(t, "app.txt", "keep")), types.OCILayerZStd)
	original := rawManifest(t, img)
	for _, dest := range []string{
		"docker.io/library/rsshub:latest",
		"demo-registry.cn-hangzhou.cr.aliyuncs.com/ytoken/rsshub:latest",
	} {
		got, err := prepareForRegistry(mustTag(t, dest), img)
		if err != nil {
			t.Fatal(err)
		}
		if rawManifest(t, got) != original {
			t.Fatalf("%s 改写了清单", dest)
		}
	}
}

func TestPrepareForRegistryLeavesGzipImage(t *testing.T) {
	layer, err := tarball.LayerFromReader(bytes.NewReader(tarBytes(t, "app.txt", "gzip")), tarball.WithMediaType(types.OCILayer))
	if err != nil {
		t.Fatal(err)
	}
	img := imageWithLayers(t, layer, types.OCILayer)
	original := rawManifest(t, img)
	ref := mustTag(t, "registry.cn-hangzhou.aliyuncs.com/ytoken/rsshub:latest")
	got, err := prepareForRegistry(ref, img)
	if err != nil {
		t.Fatal(err)
	}
	if rawManifest(t, got) != original {
		t.Fatal("gzip 镜像清单被改写")
	}
}

func TestPrepareForRegistryErrorsWhenZstdCannotBeRead(t *testing.T) {
	layer := &blobLayer{
		mt:         types.OCILayerZStd,
		compressed: []byte("not-zstd"),
		diffID:     mustHash(t, "sha256:daff148274c7d28548185931ff9a49e338c478ef633f0976e1c8bbe0ba6861a2"),
		digest:     mustHash(t, "sha256:fb16e77db25b300c89dad824934db669056184575d0ceeb03b3804fdb6cc98c7"),
	}
	img := imageWithLayers(t, layer, types.OCILayerZStd)
	ref := mustTag(t, "registry.cn-hangzhou.aliyuncs.com/ytoken/rsshub:latest")
	_, err := prepareForRegistry(ref, img)
	if err == nil || !strings.Contains(err.Error(), "转换 zstd 层失败") {
		t.Fatalf("err = %v", err)
	}
}

type blobLayer struct {
	mt         types.MediaType
	compressed []byte
	diffID     v1.Hash
	digest     v1.Hash
}

func (l *blobLayer) Digest() (v1.Hash, error) { return l.digest, nil }
func (l *blobLayer) DiffID() (v1.Hash, error) { return l.diffID, nil }
func (l *blobLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.compressed)), nil
}
func (l *blobLayer) Uncompressed() (io.ReadCloser, error) {
	if strings.HasSuffix(string(l.mt), "+zstd") {
		dec, err := zstd.NewReader(bytes.NewReader(l.compressed))
		if err != nil {
			return nil, err
		}
		return &zstdReadCloser{dec}, nil
	}
	return nil, errors.New("未实现")
}

type zstdReadCloser struct{ *zstd.Decoder }

func (z *zstdReadCloser) Close() error {
	z.Decoder.Close()
	return nil
}

func (l *blobLayer) Size() (int64, error) { return int64(len(l.compressed)), nil }
func (l *blobLayer) MediaType() (types.MediaType, error) {
	return l.mt, nil
}

func zstdLayer(t *testing.T, tarContent []byte) v1.Layer {
	t.Helper()
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Write(tarContent); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	diffID, _, err := v1.SHA256(bytes.NewReader(tarContent))
	if err != nil {
		t.Fatal(err)
	}
	digest, _, err := v1.SHA256(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return &blobLayer{
		mt:         types.OCILayerZStd,
		compressed: buf.Bytes(),
		diffID:     diffID,
		digest:     digest,
	}
}

func imageWithLayers(t *testing.T, layer v1.Layer, mt types.MediaType) v1.Image {
	t.Helper()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer:       layer,
		MediaType:   mt,
		Annotations: map[string]string{"org.opencontainers.image.title": "app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func tarBytes(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte(content)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustTag(t *testing.T, ref string) name.Reference {
	t.Helper()
	tag, err := name.NewTag(ref)
	if err != nil {
		t.Fatal(err)
	}
	return tag
}

func rawConfig(t *testing.T, img v1.Image) []byte {
	t.Helper()
	raw, err := img.RawConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func rawManifest(t *testing.T, taggable interface{ RawManifest() ([]byte, error) }) string {
	t.Helper()
	raw, err := taggable.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func imageManifest(t *testing.T, img v1.Image) *v1.Manifest {
	t.Helper()
	manifest, err := img.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func layerDigest(t *testing.T, img v1.Image, index int) v1.Hash {
	t.Helper()
	return imageManifest(t, img).Layers[index].Digest
}

func mustCompressed(t *testing.T, layer v1.Layer) io.ReadCloser {
	t.Helper()
	rc, err := layer.Compressed()
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

func readAll(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gunzip(t *testing.T, body []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
