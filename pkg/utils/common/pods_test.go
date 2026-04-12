package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortPodsByName(t *testing.T) {
	tests := []struct {
		name     string
		input    core_v1.PodList
		expected []string
	}{
		{
			name: "Unsorted pods",
			input: core_v1.PodList{
				Items: []core_v1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod-c"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-a"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-b"}},
				},
			},
			expected: []string{"pod-a", "pod-b", "pod-c"},
		},
		{
			name: "Already sorted pods",
			input: core_v1.PodList{
				Items: []core_v1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod-1"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-2"}},
				},
			},
			expected: []string{"pod-1", "pod-2"},
		},
		{
			name: "Empty pod list",
			input: core_v1.PodList{
				Items: []core_v1.Pod{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SortPodsByName(tt.input)
			resultNames := []string{}
			for _, pod := range result.Items {
				resultNames = append(resultNames, pod.Name)
			}
			assert.Equal(t, tt.expected, resultNames)
		})
	}
}

func TestSortPodsByNameReverse(t *testing.T) {
	tests := []struct {
		name     string
		input    core_v1.PodList
		expected []string
	}{
		{
			name: "Unsorted pods reverse",
			input: core_v1.PodList{
				Items: []core_v1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod-a"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-c"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-b"}},
				},
			},
			expected: []string{"pod-c", "pod-b", "pod-a"},
		},
		{
			name: "Already reverse sorted pods",
			input: core_v1.PodList{
				Items: []core_v1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod-2"}},
					{ObjectMeta: v1.ObjectMeta{Name: "pod-1"}},
				},
			},
			expected: []string{"pod-2", "pod-1"},
		},
		{
			name: "Empty pod list",
			input: core_v1.PodList{
				Items: []core_v1.Pod{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SortPodsByNameReverse(tt.input)
			resultNames := []string{}
			for _, pod := range result.Items {
				resultNames = append(resultNames, pod.Name)
			}
			assert.Equal(t, tt.expected, resultNames)
		})
	}
}
