package protocol

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"modelcheck/trace"
)

const tsharkAdapterVersion = "1"

const (
	tsharkMaxPackets = 2000
	tsharkMaxLayers  = 16
	tsharkMaxRecords = 420003
)

// TSharkJSONInput describes a caller-produced projection, not raw TShark output.
type TSharkJSONInput struct {
	Artifact      Input
	CaptureID     string
	TSharkVersion string
	Profile       string
	ExportSHA256  string
	ExportStatus  string
}

type tsharkField struct{ layer, name, value string }
type tsharkLayer struct {
	name        string
	fields      []tsharkField
	omitted     []string
	unsupported bool
	ordinal     int
}
type tsharkPacket struct{ layers []tsharkLayer }

var (
	tsharkCaptureRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	tsharkVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	tsharkLayerRE   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
	tsharkDecimalRE = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	tsharkEtherRE   = regexp.MustCompile(`^0x[0-9a-f]{4}$`)
)

// The order in these slices is the frozen profile order and is also the order
// used in omission locations. It is deliberately not derived from a map.
var tsharkProfileFields = map[string][]string{
	"eth":  {"eth.type"},
	"ip":   {"ip.proto"},
	"ipv6": {"ipv6.nxt"},
	"tcp":  {"tcp.srcport", "tcp.dstport"},
	"udp":  {"udp.srcport", "udp.dstport"},
	"dns":  {"dns.flags.response", "dns.qry.type", "dns.resp.type"},
}

var tsharkUnsupported = map[string]bool{
	"arp": true, "http": true, "http2": true, "icmp": true,
	"icmpv6": true, "quic": true, "tls": true,
}

// AnalyzeTShark returns canonical v1 presence facts from a pinned offline projection.
// It does not execute TShark or establish capture identity or completeness.
func AnalyzeTShark(input TSharkJSONInput) ([]byte, error) {
	return analyzeTSharkWithBudget(input, defaultBudget)
}

func analyzeTSharkWithBudget(input TSharkJSONInput, limits budget) ([]byte, error) {
	snapshot, release, err := begin(input.Artifact, input.ExportSHA256, func() bool {
		return tsharkCaptureRE.MatchString(input.CaptureID) &&
			tsharkVersionRE.MatchString(input.TSharkVersion) &&
			input.TSharkVersion == "4.4.10" &&
			input.Profile == "tshark-4.4.10-core-1" &&
			(input.ExportStatus == "complete" || input.ExportStatus == "truncated" || input.ExportStatus == "unknown")
	}, []string{input.CaptureID, input.TSharkVersion, input.Profile, input.ExportSHA256, input.ExportStatus})
	if err != nil {
		return nil, err
	}
	defer release()

	value, err := decodeJSON(snapshot)
	if err != nil {
		return nil, err
	}
	// Schema validation must win over the structured-secret row. planTShark
	// performs all envelope, profile, and scalar checks without graph records.
	packets, err := planTShark(value)
	if err != nil {
		return nil, err
	}
	if err := scanStructured(value); err != nil {
		return nil, err
	}

	inputID := tupleHash("tshark-input/1", input.Artifact.Path, input.ExportSHA256, input.CaptureID, input.TSharkVersion, input.Profile)
	records, outputBound := estimateTSharkPlan(input, packets, inputID)
	if err := checkBudget(records, outputBound, limits); err != nil {
		return nil, err
	}

	d := newDocument(trace.Schema)
	producer := trace.Producer{Name: "mcd-tshark-projection-adapter", Version: tsharkAdapterVersion}
	addEvidence := func(subject string, p trace.Provenance, ids *[]string) {
		id := "evidence.decoded." + tupleHash("tshark-evidence/1", subject)
		d.Evidence = append(d.Evidence, trace.Evidence{ID: id, Subject: subject, Level: trace.Decoded, Provenance: p})
		*ids = append(*ids, id)
	}

	for packetIndex, packet := range packets {
		for _, layer := range packet.layers {
			if layer.name == "frame" {
				continue
			}
			layerAnchor := tsharkLayerAnchor(input, packetIndex+1, layer)
			provenance := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: layerAnchor, Producer: producer}
			if layer.unsupported {
				appendTSharkGap(&d.Gaps, inputID, "tshark.unsupported_dissector", layerAnchor,
					"the profile recognises this dissector but does not map it", provenance)
				continue
			}

			layerID := tupleHash("tshark-layer/1", inputID, strconv.Itoa(packetIndex+1), strconv.Itoa(layer.ordinal), layer.name)
			nodeID := "protocol-message." + layerID
			evidenceIDs := []string{}
			addEvidence("node:"+nodeID, provenance, &evidenceIDs)
			d.Nodes = append(d.Nodes, trace.Node{ID: nodeID, Kind: trace.ProtocolMessage, Label: "TShark layer " + layer.name, Provenance: provenance, Evidence: evidenceIDs})

			if len(layer.omitted) != 0 {
				location := layerAnchor + "&omitted=" + strings.Join(layer.omitted, ",")
				appendTSharkGap(&d.Gaps, inputID, "tshark.permitted_fields_omitted", location,
					"one or more permitted fields were absent from the projection", provenance)
			}
			for _, field := range layer.fields {
				fieldID := tupleHash("tshark-field/1", layerID, field.name)
				fieldNodeID := "binary-field." + fieldID
				fieldAnchor := layerAnchor + "&field=" + field.name
				fieldProvenance := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: fieldAnchor, Producer: producer}

				fieldEvidence := []string{}
				addEvidence("node:"+fieldNodeID, fieldProvenance, &fieldEvidence)
				d.Nodes = append(d.Nodes, trace.Node{ID: fieldNodeID, Kind: trace.BinaryField, Label: "TShark field " + field.name, Provenance: fieldProvenance, Evidence: fieldEvidence})

				edgeID := "edge.derived-from." + tupleHash("tshark-derived-from/1", fieldNodeID, nodeID)
				edgeEvidence := []string{}
				addEvidence("edge:"+edgeID, fieldProvenance, &edgeEvidence)
				d.Edges = append(d.Edges, trace.Edge{ID: edgeID, Kind: trace.DerivedFrom, Source: fieldNodeID, Target: nodeID, Provenance: fieldProvenance, Evidence: edgeEvidence})
			}
		}
	}

	exportAnchor := tsharkExportAnchor(input)
	exportProvenance := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: exportAnchor, Producer: producer}
	appendTSharkGap(&d.Gaps, inputID, "tshark.producer_unverified", exportAnchor,
		"the adapter did not run TShark or verify the asserted producer", exportProvenance)
	appendTSharkGap(&d.Gaps, inputID, "tshark.capture_identity_unverified", exportAnchor,
		"capture bytes were not supplied to verify the asserted capture identity", exportProvenance)
	if input.ExportStatus == "truncated" || input.ExportStatus == "unknown" {
		kind := "tshark.export_" + input.ExportStatus
		reason := "the caller declared the export truncated"
		if input.ExportStatus == "unknown" {
			kind = "tshark.export_status_unknown"
			reason = "the caller did not establish export completeness"
		}
		appendTSharkGap(&d.Gaps, inputID, kind, exportAnchor, reason, exportProvenance)
	}
	return finish(d)
}

func tsharkExportAnchor(input TSharkJSONInput) string {
	return fmt.Sprintf("tshark:%s#path=%s&export=%s&tshark=%s&profile=%s", input.CaptureID, input.Artifact.Path, input.ExportSHA256, input.TSharkVersion, input.Profile)
}

func tsharkLayerAnchor(input TSharkJSONInput, packet int, layer tsharkLayer) string {
	return fmt.Sprintf("tshark:%s#path=%s&packet=%d&layer=%d-%s&tshark=%s&profile=%s", input.CaptureID, input.Artifact.Path, packet, layer.ordinal, layer.name, input.TSharkVersion, input.Profile)
}

func appendTSharkGap(gaps *[]trace.Gap, inputID, kind, location, reason string, provenance trace.Provenance) {
	id := "gap." + kind + "." + tupleHash("tshark-gap/1", inputID, kind, location)
	*gaps = append(*gaps, trace.Gap{ID: id, Kind: kind, Reference: location, Reason: reason, Provenance: provenance})
}

// estimateTSharkPlan returns concrete record and canonical JSON upper bounds.
// It constructs every generated ID, anchor, label, reference, provenance, and
// evidence subject before any trace.Node/Edge/Evidence/Gap is allocated.
func estimateTSharkPlan(input TSharkJSONInput, packets []tsharkPacket, inputID string) (uint64, uint64) {
	producer := trace.Producer{Name: "mcd-tshark-projection-adapter", Version: tsharkAdapterVersion}
	exportAnchor := tsharkExportAnchor(input)
	exportProvenance := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: exportAnchor, Producer: producer}
	// Reserve conservative parser/admission slack in the output bound. The
	// shared budget checker then applies the required working estimate
	// 2*output + 256*records + 2 MiB.
	output := uint64(2 << 20)
	records := uint64(2)
	gapBounds := make([]uint64, 0, 3)
	gapBounds = append(gapBounds,
		tsharkGapBound("tshark.producer_unverified", exportAnchor, exportProvenance, "the adapter did not run TShark or verify the asserted producer"),
		tsharkGapBound("tshark.capture_identity_unverified", exportAnchor, exportProvenance, "capture bytes were not supplied to verify the asserted capture identity"))
	if input.ExportStatus == "truncated" || input.ExportStatus == "unknown" {
		kind := "tshark.export_" + input.ExportStatus
		reason := "the caller declared the export truncated"
		if input.ExportStatus == "unknown" {
			kind = "tshark.export_status_unknown"
			reason = "the caller did not establish export completeness"
		}
		gapBounds = append(gapBounds, tsharkGapBound(kind, exportAnchor, exportProvenance, reason))
		records++
	}
	for packetIndex, packet := range packets {
		for _, layer := range packet.layers {
			if layer.name == "frame" {
				continue
			}
			anchor := tsharkLayerAnchor(input, packetIndex+1, layer)
			prov := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: anchor, Producer: producer}
			if layer.unsupported {
				records++
				gapBounds = append(gapBounds, tsharkGapBound("tshark.unsupported_dissector", anchor, prov))
				continue
			}
			layerID := tupleHash("tshark-layer/1", inputID, strconv.Itoa(packetIndex+1), strconv.Itoa(layer.ordinal), layer.name)
			nodeID := "protocol-message." + layerID
			evidenceID := "evidence.decoded." + tupleHash("tshark-evidence/1", "node:"+nodeID)
			records += 2
			output += tsharkNodeBound(nodeID, "ProtocolMessage", "TShark layer "+layer.name, prov, evidenceID)
			if len(layer.omitted) != 0 {
				location := anchor + "&omitted=" + strings.Join(layer.omitted, ",")
				records++
				gapBounds = append(gapBounds, tsharkGapBound("tshark.permitted_fields_omitted", location, prov, "one or more permitted fields were absent from the projection"))
			}
			for _, field := range layer.fields {
				fieldID := tupleHash("tshark-field/1", layerID, field.name)
				fieldNodeID := "binary-field." + fieldID
				fieldAnchor := anchor + "&field=" + field.name
				fieldProv := trace.Provenance{ContentSHA256: input.ExportSHA256, Anchor: fieldAnchor, Producer: producer}
				fieldEvidence := "evidence.decoded." + tupleHash("tshark-evidence/1", "node:"+fieldNodeID)
				edgeID := "edge.derived-from." + tupleHash("tshark-derived-from/1", fieldNodeID, nodeID)
				edgeEvidence := "evidence.decoded." + tupleHash("tshark-evidence/1", "edge:"+edgeID)
				records += 4
				output += tsharkNodeBound(fieldNodeID, "BinaryField", "TShark field "+field.name, fieldProv, fieldEvidence)
				output += tsharkEdgeBound(edgeID, fieldNodeID, nodeID, fieldProv, edgeEvidence)
			}
		}
	}
	for _, bound := range gapBounds {
		output += bound
	}
	return records, tsharkDocumentBound(output)
}

func tsharkDocumentBound(recordsBound uint64) uint64 {
	// Document object keys, four array wrappers, commas, and the schema string.
	return recordsBound + strBound("schema") + strBound(trace.Schema) + strBound("nodes") + strBound("edges") + strBound("evidence") + strBound("gaps") + 32
}

func tsharkNodeBound(id, kind, label string, p trace.Provenance, evidence string) uint64 {
	return objectBound(fieldBound("id", id), fieldBound("kind", kind), fieldBound("label", label), fieldBound("provenance", provenanceBound(p)), fieldBound("evidence", arrayBound(strBound(evidence)))) + tsharkEvidenceBound(evidence, "node:"+id, p)
}

func tsharkEdgeBound(id, source, target string, p trace.Provenance, evidence string) uint64 {
	return objectBound(fieldBound("id", id), fieldBound("kind", "derived_from"), fieldBound("source", source), fieldBound("target", target), fieldBound("provenance", provenanceBound(p)), fieldBound("evidence", arrayBound(strBound(evidence)))) + tsharkEvidenceBound(evidence, "edge:"+id, p)
}

func tsharkEvidenceBound(id, subject string, p trace.Provenance) uint64 {
	return objectBound(fieldBound("id", id), fieldBound("subject", subject), fieldBound("level", "decoded"), fieldBound("provenance", provenanceBound(p)))
}

func tsharkGapBound(kind, location string, p trace.Provenance, reasons ...string) uint64 {
	reason := "the profile recognises this dissector but does not map it"
	if len(reasons) != 0 {
		reason = reasons[0]
	}
	return objectBound(fieldBound("id", "gap."+kind+"."+tupleHash("tshark-gap/1", "planned-input", kind, location)), fieldBound("kind", kind), fieldBound("reference", location), fieldBound("reason", reason), fieldBound("provenance", provenanceBound(p)))
}

func provenanceBound(p trace.Provenance) uint64 {
	fields := []uint64{fieldBound("content_sha256", p.ContentSHA256), fieldBound("anchor", p.Anchor), fieldBound("producer", objectBound(fieldBound("name", p.Producer.Name), fieldBound("version", p.Producer.Version)))}
	return objectBound(fields...)
}
func fieldBound(key string, value any) uint64 {
	var n uint64
	switch v := value.(type) {
	case string:
		n = strBound(v)
	case uint64:
		n = v
	}
	return strBound(key) + 1 + n
}
func objectBound(fields ...uint64) uint64 {
	// Reserve framing, indentation and array separators for each small object.
	var n uint64 = 256
	for i, field := range fields {
		if i != 0 {
			n++
		}
		n += field
	}
	return n
}
func arrayBound(items ...uint64) uint64 {
	var n uint64 = 2
	for i, item := range items {
		if i != 0 {
			n++
		}
		n += item
	}
	return n
}
func strBound(s string) uint64 { return uint64(len(s))*6 + 2 }

func planTShark(v any) ([]tsharkPacket, error) {
	a, ok := v.([]any)
	if !ok || len(a) < 1 || len(a) > tsharkMaxPackets {
		return nil, rule("schema_invalid", "packets")
	}
	out := make([]tsharkPacket, 0, len(a))
	for i, x := range a {
		o, ok := x.(map[string]any)
		if !ok || len(o) != 1 {
			return nil, rule("schema_invalid", fmt.Sprintf("packets[%d]", i))
		}
		lv, ok := o["layers"].([]any)
		if !ok || len(lv) < 2 || len(lv) > tsharkMaxLayers {
			return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers", i))
		}
		packet := tsharkPacket{}
		chain := make([]string, 0, len(lv)-1)
		for j, y := range lv {
			lo, ok := y.(map[string]any)
			if !ok || len(lo) != 2 {
				return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d]", i, j))
			}
			name, ok := lo["name"].(string)
			if !ok || !tsharkLayerRE.MatchString(name) {
				return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].name", i, j))
			}
			if j == 0 && name != "frame" {
				return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].name", i, j))
			}
			if j > 0 && name == "frame" {
				return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].name", i, j))
			}
			fields, ok := lo["fields"].(map[string]any)
			if !ok {
				return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
			}
			layer := tsharkLayer{name: name, ordinal: j}
			if j == 0 {
				if len(fields) != 1 {
					return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
				}
				protocols, ok := fields["frame.protocols"].(string)
				if !ok {
					return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
				}
				layer.fields = append(layer.fields, tsharkField{layer: name, name: "frame.protocols", value: protocols})
			} else if tsharkUnsupported[name] {
				if len(fields) != 0 {
					return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
				}
				layer.unsupported = true
				chain = append(chain, name)
			} else {
				allowed, exists := tsharkProfileFields[name]
				if !exists {
					return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].name", i, j))
				}
				for _, fieldName := range allowed {
					raw, present := fields[fieldName]
					if !present {
						layer.omitted = append(layer.omitted, fieldName)
						continue
					}
					fieldValue, ok := raw.(string)
					if !ok || !validTsharkValue(name, fieldName, fieldValue) {
						return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
					}
					layer.fields = append(layer.fields, tsharkField{layer: name, name: fieldName, value: fieldValue})
				}
				if len(fields) != len(layer.fields) {
					return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
				}
				for fieldName := range fields {
					known := false
					for _, allowedName := range allowed {
						if fieldName == allowedName {
							known = true
							break
						}
					}
					if !known {
						return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[%d].fields", i, j))
					}
				}
				chain = append(chain, name)
			}
			packet.layers = append(packet.layers, layer)
		}
		frame := packet.layers[0].fields[0].value
		if frame != strings.Join(chain, ":") || len(chain) < 1 || len(chain) > 15 {
			return nil, rule("schema_invalid", fmt.Sprintf("packets[%d].layers[0].fields", i))
		}
		out = append(out, packet)
	}
	return out, nil
}

func validTsharkValue(layer, field, value string) bool {
	if layer == "eth" {
		return tsharkEtherRE.MatchString(value)
	}
	if layer == "dns" && field == "dns.flags.response" {
		return value == "0" || value == "1"
	}
	if !tsharkDecimalRE.MatchString(value) {
		return false
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	if layer == "ip" || layer == "ipv6" {
		return n <= 255
	}
	return n <= 65535
}
