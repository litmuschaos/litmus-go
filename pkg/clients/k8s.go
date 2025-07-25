package clients

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/litmuschaos/litmus-go/pkg/utils/retry"
)

var (
	defaultTimeout = 180
	defaultDelay   = 2
)

func (clients *ClientSets) GetPod(namespace, name string, timeout, delay int) (*core_v1.Pod, error) {
	var (
		pod *core_v1.Pod
		err error
	)

	if err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			pod, err = clients.KubeClient.CoreV1().Pods(namespace).Get(context.Background(), name, v1.GetOptions{})
			return err
		}); err != nil {
		return nil, err
	}

	return pod, nil
}

func (clients *ClientSets) GetAllPod(namespace string) (*core_v1.PodList, error) {
	var (
		pods *core_v1.PodList
		err  error
	)

	if err := retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			pods, err = clients.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{})
			return err
		}); err != nil {
		return nil, err
	}

	return pods, nil
}

func (clients *ClientSets) ListPods(namespace, labels string) (*core_v1.PodList, error) {
	var (
		pods *core_v1.PodList
		err  error
	)

	if err := retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			pods, err = clients.KubeClient.CoreV1().Pods(namespace).List(context.Background(), v1.ListOptions{
				LabelSelector: labels,
			})
			return err
		}); err != nil {
		return nil, err
	}

	return pods, nil
}

func (clients *ClientSets) GetService(namespace, name string) (*core_v1.Service, error) {
	var (
		pod *core_v1.Service
		err error
	)

	if err := retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			pod, err = clients.KubeClient.CoreV1().Services(namespace).Get(context.Background(), name, v1.GetOptions{})
			return err
		}); err != nil {
		return nil, err
	}

	return pod, nil
}

func (clients *ClientSets) CreatePod(namespace string, pod *core_v1.Pod) error {
	return retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.KubeClient.CoreV1().Pods(namespace).Create(context.Background(), pod, v1.CreateOptions{})
			return err
		})
}

func (clients *ClientSets) CreateDeployment(namespace string, deploy *appsv1.Deployment) error {
	return retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.KubeClient.AppsV1().Deployments(namespace).Create(context.Background(), deploy, v1.CreateOptions{})
			return err
		})
}

func (clients *ClientSets) CreateService(namespace string, svc *core_v1.Service) error {
	return retry.
		Times(uint(defaultTimeout / defaultDelay)).
		Wait(time.Duration(defaultDelay) * time.Second).
		Try(func(attempt uint) error {
			_, err := clients.KubeClient.CoreV1().Services(namespace).Create(context.Background(), svc, v1.CreateOptions{})
			return err
		})
}

func (clients *ClientSets) GetNode(name string, timeout, delay int) (*core_v1.Node, error) {
	var (
		node *core_v1.Node
		err  error
	)

	if err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			node, err = clients.KubeClient.CoreV1().Nodes().Get(context.Background(), name, v1.GetOptions{})
			return err
		}); err != nil {
		return nil, err
	}

	return node, nil
}

func (clients *ClientSets) GetAllNode(timeout, delay int) (*core_v1.NodeList, error) {
	var (
		nodes *core_v1.NodeList
		err   error
	)

	if err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			nodes, err = clients.KubeClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
			return err
		}); err != nil {
		return nil, err
	}

	return nodes, nil
}

func (clients *ClientSets) ListNode(labels string, timeout, delay int) (*core_v1.NodeList, error) {
	var (
		nodes *core_v1.NodeList
		err   error
	)

	if err := retry.
		Times(uint(timeout / delay)).
		Wait(time.Duration(delay) * time.Second).
		Try(func(attempt uint) error {
			nodes, err = clients.KubeClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{
				LabelSelector: labels,
			})
			return err
		}); err != nil {
		return nil, err
	}

	return nodes, nil
}
