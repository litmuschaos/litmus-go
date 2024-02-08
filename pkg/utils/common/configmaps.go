package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/litmuschaos/litmus-go/pkg/cerrors"
	"github.com/litmuschaos/litmus-go/pkg/clients"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/palantir/stacktrace"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// DeleteConfigMap deletes the specified configmap
func DeleteConfigMap(configMapName, namespace string, clients clients.ClientSets) error {
	if err := clients.KubeClient.CoreV1().ConfigMaps(namespace).Delete(context.Background(), configMapName, v1.DeleteOptions{}); err != nil {
		return cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Target: fmt.Sprintf("{configMapName: %s, namespace: %s}", configMapName, namespace), Reason: fmt.Sprintf("failed to delete configmap: %s", err.Error())}
	}
	return nil
}

// CreateConfigMapFromFile creates the configmap
func CreateConfigMapFromFile(namespace, filePath string, clients clients.ClientSets, chaosDetails *types.ChaosDetails) (string, error) {
	scriptConfig := &corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: "litmus-go-cm-",
			Namespace:    namespace,
		},
		Data: map[string]string{},
	}

	if err := addKeyFromFileToConfigMap(scriptConfig, filePath); err != nil {
		return "", stacktrace.Propagate(err, "could not add script to configmap")
	}

	cm, err := clients.KubeClient.CoreV1().ConfigMaps(namespace).Create(context.Background(), scriptConfig, v1.CreateOptions{})
	if err != nil {
		return "", cerrors.Error{ErrorCode: cerrors.ErrorTypeGeneric, Reason: fmt.Sprintf("unable to create script configmap: %s", err.Error())}
	}

	return cm.Name, nil
}

func addKeyFromFileToConfigMap(configMap *corev1.ConfigMap, filePath string) error {
	// Using filename as key
	_, filename := filepath.Split(filePath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if utf8.Valid(data) {
		return addKeyFromLiteralToConfigMap(configMap, filename, string(data))
	}
	err = validateNewConfigMap(configMap, filename)
	if err != nil {
		return err
	}
	configMap.BinaryData[filename] = data

	return nil
}

func addKeyFromLiteralToConfigMap(configMap *corev1.ConfigMap, keyName, data string) error {
	err := validateNewConfigMap(configMap, keyName)
	if err != nil {
		return err
	}
	configMap.Data[keyName] = data

	return nil
}

func validateNewConfigMap(configMap *corev1.ConfigMap, keyName string) error {
	if errs := validation.IsConfigMapKey(keyName); len(errs) > 0 {
		return fmt.Errorf("%q is not a valid key name for a ConfigMap: %s", keyName, strings.Join(errs, ","))
	}
	if _, exists := configMap.Data[keyName]; exists {
		return fmt.Errorf("cannot add key %q, another key by that name already exists in Data for ConfigMap %q", keyName, configMap.Name)
	}
	if _, exists := configMap.BinaryData[keyName]; exists {
		return fmt.Errorf("cannot add key %q, another key by that name already exists in BinaryData for ConfigMap %q", keyName, configMap.Name)
	}

	return nil
}
