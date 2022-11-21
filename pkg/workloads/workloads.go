package workloads

import (
	"context"
	"github.com/litmuschaos/litmus-go/pkg/clients"
)

type Workloads interface {
	GetPodsFromDeployment(deployName string) ([]string, error)
	GetPodsFromDaemonSet(daemonName string) ([]string, error)
	GetPodsFromStatefulSet(stsName string) ([]string, error)
	GetPodsFromDeploymentConfig(dcName string) ([]string, error)
	GetPodsFromRollout(roName string) ([]string, error)
}

type Workload struct {
	ctx       context.Context
	namespace string
	client    clients.ClientSets
}

func NewWorkload(ctx context.Context, namespace string, client clients.ClientSets) Workloads {
	return &Workload{
		ctx:       ctx,
		namespace: namespace,
		client:    client,
	}
}

func (w *Workload) GetPodsFromDeployment(deployName string) ([]string, error) {
	var pods []string

	allRS, err := getAllReplicaSets(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	allPods, err := getAllPods(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	for _, rs := range getRSFromDeployment(allRS, deployName) {
		pods = append(pods, getPodsFromRS(allPods, rs)...)
	}

	return pods, nil
}

func (w *Workload) GetPodsFromDaemonSet(daemonName string) ([]string, error) {
	var pods []string

	allPods, err := getAllPods(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	for _, p := range allPods.Items {
		for _, owner := range p.OwnerReferences {
			if owner.Name == daemonName && owner.Kind == DaemonSet {
				pods = append(pods, p.Name)
				break
			}
		}
	}
	return pods, nil
}

func (w *Workload) GetPodsFromStatefulSet(stsName string) ([]string, error) {
	var pods []string

	allPods, err := getAllPods(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	for _, p := range allPods.Items {
		for _, owner := range p.OwnerReferences {
			if owner.Name == stsName && owner.Kind == StatefulSet {
				pods = append(pods, p.Name)
				break
			}
		}
	}
	return pods, nil
}

func (w *Workload) GetPodsFromDeploymentConfig(dcName string) ([]string, error) {
	var pods []string

	allRC, err := getAllReplicaControllers(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	allPods, err := getAllPods(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	for _, rc := range getRCFromDeploymentConfig(allRC, dcName) {
		pods = append(pods, getPodsFromRC(allPods, rc)...)
	}

	return pods, nil
}

func (w *Workload) GetPodsFromRollout(roName string) ([]string, error) {
	var pods []string

	allRS, err := getAllReplicaSets(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	allPods, err := getAllPods(w.namespace, w.client)
	if err != nil {
		return nil, err
	}

	for _, rs := range getRSFromRollout(allRS, roName) {
		pods = append(pods, getPodsFromRS(allPods, rs)...)
	}

	return pods, nil
}
