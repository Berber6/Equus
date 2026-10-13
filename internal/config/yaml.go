package config

import (
	"errors"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var environmentReference = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

// decodeYAMLPatch reads one YAML document with ${VAR} expansion and strict
// field checking, then applies it over base.
func decodeYAMLPatch(reader io.Reader, base Config) (Config, error) {
	decoder := yaml.NewDecoder(reader)

	var document yaml.Node
	if err := decoder.Decode(&document); errors.Is(err, io.EOF) {
		return base, nil
	} else if err != nil {
		return Config{}, err
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, err
		}
		return Config{}, errors.New("multiple YAML documents are not supported")
	}

	if len(document.Content) == 0 {
		return base, nil
	}
	if err := expandEnvironment(document.Content[0]); err != nil {
		return Config{}, err
	}

	strict := yaml.NewDecoder(nodeReader(document.Content[0]))
	strict.KnownFields(true)
	var patch configPatch
	if err := strict.Decode(&patch); err != nil {
		return Config{}, err
	}
	return patch.apply(base), nil
}

func nodeReader(node *yaml.Node) io.Reader {
	return strings.NewReader(encodeNode(node))
}

func encodeNode(node *yaml.Node) string {
	var builder strings.Builder
	encoder := yaml.NewEncoder(&builder)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return ""
	}
	encoder.Close()
	return builder.String()
}

// expandEnvironment substitutes ${VAR} references in scalar node values.
func expandEnvironment(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			if err := expandEnvironment(node.Content[index+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := expandEnvironment(child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		expanded, err := expandScalar(node.Value)
		if err != nil {
			return err
		}
		node.Value = expanded
	}
	return nil
}

func expandScalar(value string) (string, error) {
	references := environmentReference.FindAllString(value, -1)
	if len(references) == 0 {
		return value, nil
	}
	expanded := value
	for _, reference := range references {
		name := reference[2 : len(reference)-1]
		resolved, ok := lookupEnv(name)
		if !ok {
			return "", errEnvMissing(name)
		}
		expanded = strings.ReplaceAll(expanded, reference, resolved)
	}
	return expanded, nil
}
