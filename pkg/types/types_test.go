package types

import (
	"testing"
)

func TestGetTargets_UnionModeDefault(t *testing.T) {
	// Test that union is the default mode when not specified
	targets := "deployment:default:[app=nginx,tier=frontend]"
	result := GetTargets(targets)

	if len(result) != 1 {
		t.Errorf("Expected 1 AppDetail, got %d", len(result))
	}

	if result[0].LabelMatchMode != "union" {
		t.Errorf("Expected default LabelMatchMode to be 'union', got '%s'", result[0].LabelMatchMode)
	}

	if result[0].Kind != "deployment" {
		t.Errorf("Expected Kind to be 'deployment', got '%s'", result[0].Kind)
	}

	if result[0].Namespace != "default" {
		t.Errorf("Expected Namespace to be 'default', got '%s'", result[0].Namespace)
	}

	if len(result[0].Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(result[0].Labels))
	}
}

func TestGetTargets_ExplicitUnionMode(t *testing.T) {
	// Test explicit union mode
	targets := "statefulset:prod:[app=postgres,role=primary]:union"
	result := GetTargets(targets)

	if len(result) != 1 {
		t.Errorf("Expected 1 AppDetail, got %d", len(result))
	}

	if result[0].LabelMatchMode != "union" {
		t.Errorf("Expected LabelMatchMode to be 'union', got '%s'", result[0].LabelMatchMode)
	}

	if result[0].Kind != "statefulset" {
		t.Errorf("Expected Kind to be 'statefulset', got '%s'", result[0].Kind)
	}
}

func TestGetTargets_IntersectionMode(t *testing.T) {
	// Test intersection mode
	targets := "cluster:default:[cnpg.io/instanceRole=primary,cnpg.io/cluster=pg-eu]:intersection"
	result := GetTargets(targets)

	if len(result) != 1 {
		t.Errorf("Expected 1 AppDetail, got %d", len(result))
	}

	if result[0].LabelMatchMode != "intersection" {
		t.Errorf("Expected LabelMatchMode to be 'intersection', got '%s'", result[0].LabelMatchMode)
	}

	if result[0].Kind != "cluster" {
		t.Errorf("Expected Kind to be 'cluster', got '%s'", result[0].Kind)
	}

	if result[0].Namespace != "default" {
		t.Errorf("Expected Namespace to be 'default', got '%s'", result[0].Namespace)
	}

	if len(result[0].Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(result[0].Labels))
	}

	expectedLabels := []string{"cnpg.io/instanceRole=primary", "cnpg.io/cluster=pg-eu"}
	for i, label := range result[0].Labels {
		if label != expectedLabels[i] {
			t.Errorf("Expected label[%d] to be '%s', got '%s'", i, expectedLabels[i], label)
		}
	}
}

func TestGetTargets_InvalidMode(t *testing.T) {
	// Test that invalid mode falls back to union
	targets := "deployment:default:[app=nginx]:invalid"
	result := GetTargets(targets)

	if len(result) != 1 {
		t.Errorf("Expected 1 AppDetail, got %d", len(result))
	}

	// Invalid mode should fall back to union
	if result[0].LabelMatchMode != "union" {
		t.Errorf("Expected invalid mode to fall back to 'union', got '%s'", result[0].LabelMatchMode)
	}
}

func TestGetTargets_MultipleSemicolonSeparated(t *testing.T) {
	// Test multiple targets with different modes
	targets := "deployment:ns1:[app=web]:union;statefulset:ns2:[db=postgres,env=prod]:intersection"
	result := GetTargets(targets)

	if len(result) != 2 {
		t.Errorf("Expected 2 AppDetails, got %d", len(result))
	}

	// First target - union
	if result[0].LabelMatchMode != "union" {
		t.Errorf("Expected first target LabelMatchMode to be 'union', got '%s'", result[0].LabelMatchMode)
	}

	// Second target - intersection
	if result[1].LabelMatchMode != "intersection" {
		t.Errorf("Expected second target LabelMatchMode to be 'intersection', got '%s'", result[1].LabelMatchMode)
	}
}

func TestGetTargets_WithNames(t *testing.T) {
	// Test that Names parsing still works with the new field
	targets := "pod:default:[pod1,pod2,pod3]"
	result := GetTargets(targets)

	if len(result) != 1 {
		t.Errorf("Expected 1 AppDetail, got %d", len(result))
	}

	if len(result[0].Names) != 3 {
		t.Errorf("Expected 3 names, got %d", len(result[0].Names))
	}

	if len(result[0].Labels) != 0 {
		t.Errorf("Expected 0 labels when Names are provided, got %d", len(result[0].Labels))
	}

	// Default mode should still be union
	if result[0].LabelMatchMode != "union" {
		t.Errorf("Expected default LabelMatchMode to be 'union', got '%s'", result[0].LabelMatchMode)
	}
}
