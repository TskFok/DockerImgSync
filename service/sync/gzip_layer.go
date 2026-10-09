package sync

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func acrPersonalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 && port(host[i+1:]) {
		host = host[:i]
	}
	parts := strings.Split(host, ".")
	return len(parts) == 4 && parts[0] == "registry" && parts[1] != "" && parts[2] == "aliyuncs" && parts[3] == "com"
}

func port(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isZstdLayer(mt types.MediaType) bool {
	return strings.HasSuffix(string(mt), "+zstd")
}

func gzipMediaType(mt types.MediaType) types.MediaType {
	switch value := string(mt); {
	case strings.Contains(value, "nondistributable"):
		return types.OCIRestrictedLayer
	case strings.Contains(value, "vnd.docker"):
		return types.DockerLayer
	default:
		return types.OCILayer
	}
}

func prepareForRegistry(ref name.Reference, taggable remote.Taggable) (remote.Taggable, error) {
	if taggable == nil || !acrPersonalHost(ref.Context().RegistryStr()) {
		return taggable, nil
	}
	return gzipZstdLayers(taggable)
}

func gzipZstdLayers(taggable remote.Taggable) (remote.Taggable, error) {
	switch current := taggable.(type) {
	case *remote.Descriptor:
		if current.MediaType.IsIndex() {
			idx, err := current.ImageIndex()
			if err != nil {
				return nil, err
			}
			converted, _, err := convertIndex(idx)
			return converted, err
		}
		if current.MediaType.IsImage() {
			img, err := current.Image()
			if err != nil {
				return nil, err
			}
			converted, _, err := convertImage(img)
			return converted, err
		}
		return taggable, nil
	case v1.ImageIndex:
		converted, _, err := convertIndex(current)
		return converted, err
	case v1.Image:
		converted, _, err := convertImage(current)
		return converted, err
	default:
		return taggable, nil
	}
}

func convertIndex(idx v1.ImageIndex) (v1.ImageIndex, bool, error) {
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, false, fmt.Errorf("解析镜像索引失败: %w", err)
	}
	mediaType, err := idx.MediaType()
	if err != nil {
		return nil, false, fmt.Errorf("解析镜像索引失败: %w", err)
	}
	next := manifest.DeepCopy()
	next.MediaType = mediaType
	next.Manifests = make([]v1.Descriptor, 0, len(manifest.Manifests))
	images := make(map[v1.Hash]v1.Image, len(manifest.Manifests))
	indexes := make(map[v1.Hash]v1.ImageIndex, len(manifest.Manifests))
	changed := false
	opaque := 0
	for _, desc := range manifest.Manifests {
		switch {
		case desc.MediaType.IsImage():
			img, err := idx.Image(desc.Digest)
			if err != nil {
				return nil, false, fmt.Errorf("读取子镜像 %s 失败: %w", desc.Digest, err)
			}
			converted, did, err := convertImage(img)
			if err != nil {
				return nil, false, err
			}
			changed = changed || did
			child, err := describableDescriptor(converted, desc)
			if err != nil {
				return nil, false, err
			}
			next.Manifests = append(next.Manifests, child)
			images[child.Digest] = converted
		case desc.MediaType.IsIndex():
			nested, err := idx.ImageIndex(desc.Digest)
			if err != nil {
				return nil, false, fmt.Errorf("读取子索引 %s 失败: %w", desc.Digest, err)
			}
			converted, did, err := convertIndex(nested)
			if err != nil {
				return nil, false, err
			}
			changed = changed || did
			child, err := describableDescriptor(converted, desc)
			if err != nil {
				return nil, false, err
			}
			next.Manifests = append(next.Manifests, child)
			indexes[child.Digest] = converted
		default:
			opaque++
			next.Manifests = append(next.Manifests, *desc.DeepCopy())
		}
	}
	if !changed {
		return idx, false, nil
	}
	if opaque > 0 {
		return nil, false, fmt.Errorf("索引包含无法随 zstd 层一起转换的清单")
	}
	return &gzipIndex{mediaType: mediaType, manifest: next, images: images, indexes: indexes}, true, nil
}

func convertImage(img v1.Image) (v1.Image, bool, error) {
	manifest, err := img.Manifest()
	if err != nil {
		return nil, false, fmt.Errorf("解析镜像清单失败: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, false, fmt.Errorf("读取镜像层失败: %w", err)
	}
	if len(layers) != len(manifest.Layers) {
		return nil, false, fmt.Errorf("镜像层数量不一致")
	}
	next := manifest.DeepCopy()
	converted := make([]v1.Layer, len(layers))
	changed := false
	for i, layer := range layers {
		source := layerMediaType(layer, manifest.Layers[i].MediaType)
		if !isZstdLayer(source) {
			converted[i] = layer
			continue
		}
		gzipped, err := gzipLayerFrom(layer, source)
		if err != nil {
			return nil, false, err
		}
		descriptor := manifest.Layers[i].DeepCopy()
		digest, err := gzipped.Digest()
		if err != nil {
			return nil, false, fmt.Errorf("转换 zstd 层失败: %w", err)
		}
		size, err := gzipped.Size()
		if err != nil {
			return nil, false, fmt.Errorf("转换 zstd 层失败: %w", err)
		}
		descriptor.MediaType = gzipMediaType(source)
		descriptor.Digest = digest
		descriptor.Size = size
		descriptor.Data = nil
		descriptor.URLs = nil
		next.Layers[i] = *descriptor
		converted[i] = gzipped
		changed = true
	}
	if !changed {
		return img, false, nil
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, false, fmt.Errorf("生成镜像清单失败: %w", err)
	}
	byDigest := make(map[v1.Hash]v1.Layer, len(converted))
	byDiffID := make(map[v1.Hash]v1.Layer, len(converted))
	for _, layer := range converted {
		digest, err := layer.Digest()
		if err != nil {
			return nil, false, fmt.Errorf("转换 zstd 层失败: %w", err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, false, fmt.Errorf("转换 zstd 层失败: %w", err)
		}
		byDigest[digest] = layer
		byDiffID[diffID] = layer
	}
	return &gzipImage{
		base:     img,
		layers:   converted,
		manifest: next,
		raw:      raw,
		byDigest: byDigest,
		byDiffID: byDiffID,
	}, true, nil
}

func layerMediaType(layer v1.Layer, descriptor types.MediaType) types.MediaType {
	if isZstdLayer(descriptor) {
		return descriptor
	}
	mt, err := layer.MediaType()
	if err != nil {
		return descriptor
	}
	return mt
}

func gzipLayerFrom(layer v1.Layer, source types.MediaType) (v1.Layer, error) {
	fail := func(err error) error {
		digest, digestErr := layer.Digest()
		if digestErr != nil {
			return fmt.Errorf("转换 zstd 层失败: %w", err)
		}
		return fmt.Errorf("转换 zstd 层失败: %s: %w", digest, err)
	}
	diffID, err := layer.DiffID()
	if err != nil {
		return nil, fail(err)
	}
	rc, err := layer.Uncompressed()
	if err != nil {
		return nil, fail(err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := io.Copy(writer, rc); err != nil {
		return nil, fail(err)
	}
	if err := writer.Close(); err != nil {
		return nil, fail(err)
	}
	digest, _, err := v1.SHA256(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, fail(err)
	}
	return &gzippedLayer{
		body:   buf.Bytes(),
		mt:     gzipMediaType(source),
		diffID: diffID,
		digest: digest,
	}, nil
}

type describable interface {
	Digest() (v1.Hash, error)
	MediaType() (types.MediaType, error)
	Size() (int64, error)
}

func describableDescriptor(item describable, original v1.Descriptor) (v1.Descriptor, error) {
	digest, err := item.Digest()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("计算清单摘要失败: %w", err)
	}
	size, err := item.Size()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("计算清单大小失败: %w", err)
	}
	mediaType, err := item.MediaType()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("读取清单类型失败: %w", err)
	}
	desc := original.DeepCopy()
	desc.MediaType = mediaType
	desc.Digest = digest
	desc.Size = size
	desc.Data = nil
	return *desc, nil
}

type gzippedLayer struct {
	body   []byte
	mt     types.MediaType
	diffID v1.Hash
	digest v1.Hash
}

func (l *gzippedLayer) Digest() (v1.Hash, error) { return l.digest, nil }
func (l *gzippedLayer) DiffID() (v1.Hash, error) { return l.diffID, nil }
func (l *gzippedLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.body)), nil
}
func (l *gzippedLayer) Uncompressed() (io.ReadCloser, error) {
	reader, err := gzip.NewReader(bytes.NewReader(l.body))
	if err != nil {
		return nil, err
	}
	return reader, nil
}
func (l *gzippedLayer) Size() (int64, error)                { return int64(len(l.body)), nil }
func (l *gzippedLayer) MediaType() (types.MediaType, error) { return l.mt, nil }

type gzipImage struct {
	base     v1.Image
	layers   []v1.Layer
	manifest *v1.Manifest
	raw      []byte
	byDigest map[v1.Hash]v1.Layer
	byDiffID map[v1.Hash]v1.Layer
}

func (g *gzipImage) MediaType() (types.MediaType, error) { return g.base.MediaType() }
func (g *gzipImage) ConfigName() (v1.Hash, error)        { return g.base.ConfigName() }
func (g *gzipImage) ConfigFile() (*v1.ConfigFile, error) { return g.base.ConfigFile() }
func (g *gzipImage) RawConfigFile() ([]byte, error)      { return g.base.RawConfigFile() }
func (g *gzipImage) RawManifest() ([]byte, error)        { return append([]byte(nil), g.raw...), nil }
func (g *gzipImage) Manifest() (*v1.Manifest, error)     { return g.manifest.DeepCopy(), nil }
func (g *gzipImage) Digest() (v1.Hash, error)            { return partial.Digest(g) }
func (g *gzipImage) Size() (int64, error)                { return partial.Size(g) }
func (g *gzipImage) Layers() ([]v1.Layer, error) {
	out := make([]v1.Layer, len(g.layers))
	copy(out, g.layers)
	return out, nil
}
func (g *gzipImage) LayerByDigest(h v1.Hash) (v1.Layer, error) {
	layer, ok := g.byDigest[h]
	if !ok {
		return nil, fmt.Errorf("层不存在: %s", h)
	}
	return layer, nil
}
func (g *gzipImage) LayerByDiffID(h v1.Hash) (v1.Layer, error) {
	layer, ok := g.byDiffID[h]
	if !ok {
		return nil, fmt.Errorf("层不存在: %s", h)
	}
	return layer, nil
}

type gzipIndex struct {
	mediaType types.MediaType
	manifest  *v1.IndexManifest
	images    map[v1.Hash]v1.Image
	indexes   map[v1.Hash]v1.ImageIndex
}

func (g *gzipIndex) MediaType() (types.MediaType, error) { return g.mediaType, nil }
func (g *gzipIndex) IndexManifest() (*v1.IndexManifest, error) {
	return g.manifest.DeepCopy(), nil
}
func (g *gzipIndex) RawManifest() ([]byte, error) { return json.Marshal(g.manifest) }
func (g *gzipIndex) Digest() (v1.Hash, error)     { return partial.Digest(g) }
func (g *gzipIndex) Size() (int64, error)         { return partial.Size(g) }
func (g *gzipIndex) Image(h v1.Hash) (v1.Image, error) {
	img, ok := g.images[h]
	if !ok {
		return nil, fmt.Errorf("镜像不存在: %s", h)
	}
	return img, nil
}
func (g *gzipIndex) ImageIndex(h v1.Hash) (v1.ImageIndex, error) {
	idx, ok := g.indexes[h]
	if !ok {
		return nil, fmt.Errorf("索引不存在: %s", h)
	}
	return idx, nil
}
