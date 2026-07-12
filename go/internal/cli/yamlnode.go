package cli

import "gopkg.in/yaml.v3"

// scalarNode builds a plain string scalar node.
func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// setMapScalar sets key to a string value in a mapping node, adding the pair if
// the key is absent.
func setMapScalar(m *yaml.Node, key, value string) {
	if v := mapValue(m, key); v != nil {
		v.Kind = yaml.ScalarNode
		v.Tag = "!!str"
		v.Value = value
		return
	}
	appendMapEntry(m, key, scalarNode(value))
}

// setMapSequence sets key to a flow sequence of string scalars.
func setMapSequence(m *yaml.Node, key string, values []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, v := range values {
		seq.Content = append(seq.Content, scalarNode(v))
	}
	if existing := mapValue(m, key); existing != nil {
		*existing = *seq
		return
	}
	appendMapEntry(m, key, seq)
}

// appendMapEntry appends a key/value pair to a mapping node.
func appendMapEntry(m *yaml.Node, key string, value *yaml.Node) {
	if m.Kind != yaml.MappingNode {
		return
	}
	m.Content = append(m.Content, scalarNode(key), value)
}

// ensureMap returns the mapping node at key, creating an empty one if absent.
func ensureMap(root *yaml.Node, key string) *yaml.Node {
	if v := mapValue(root, key); v != nil {
		return v
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendMapEntry(root, key, m)
	return m
}

// repoEntryNode builds a repo's config block: path/default_base/compose_role and
// link_node_modules:false, matching the block bin/edit-project writes.
func repoEntryNode(name, base, role string) *yaml.Node {
	boolFalse := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("path"), scalarNode(name),
		scalarNode("default_base"), scalarNode(base),
		scalarNode("compose_role"), scalarNode(role),
		scalarNode("link_node_modules"), boolFalse,
	}}
}
