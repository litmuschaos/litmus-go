package tc

import (
	"errors"
	"fmt"
	"net"
	"os/exec"

	"github.com/litmuschaos/litmus-go/chaoslib/litmus/network_latency/ip"
	"github.com/litmuschaos/litmus-go/pkg/log"
)

func CreateDelayQdisc(PID int, latency float64, jitter float64) error {

	if PID == 0 {
		log.Error(fmt.Sprintf("[tc] Invalid PID=%d", PID))
		return errors.New("Target PID cannot be zero")
	}

	if latency <= 0 {
		log.Error(fmt.Sprintf("[tc] Invalid latency=%f", latency))
		return errors.New("Latency should be a positive value")
	}

	iface, err := ip.InterfaceName(PID)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[tc] CreateDelayQdisc: PID=%d interface=%s latency=%fs jitter=%fs", PID, iface, latency, jitter))

	tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc add dev %s root handle 1: prio", PID, iface)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())
	if err != nil {
		log.Error(string(out))
		return err
	}

	if almostZero(jitter) {
		// no jitter
		tc = fmt.Sprintf("sudo nsenter -t %d -n tc qdisc add dev %s parent 1:3 netem delay %fs", PID, iface, latency)
	} else {
		tc = fmt.Sprintf("sudo nsenter -t %d -n tc qdisc add dev %s parent 1:3 netem delay %fs %fs", PID, iface, latency, jitter)
	}
	cmd = exec.Command("/bin/bash", "-c", tc)
	out, err = cmd.CombinedOutput()
	log.Info(cmd.String())
	if err != nil {
		log.Error(string(out))
		return err
	}

	return nil
}

func AddIPFilter(PID int, IP net.IP, port int) error {
	if PID == 0 {
		log.Error(fmt.Sprintf("[tc] Invalid PID=%d", PID))
		return errors.New("Target PID cannot be zero")
	}

	if port == 0 {
		log.Error(fmt.Sprintf("[tc] Invalid Port=%d", port))
		return errors.New("Port cannot be zero")
	}

	log.Info(fmt.Sprintf("[tc] AddIPFilter: Target PID=%d, destination IP=%s, destination Port=%d", PID, IP, port))

	tc := fmt.Sprintf("sudo nsenter -t %d -n tc filter add dev eth0 protocol ip parent 1:0 prio 3 u32 match ip dst %s match ip dport %d 0xffff flowid 1:3", PID, IP, port)
	cmd := exec.Command("/bin/bash", "-c", tc)

	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())
	if err != nil {
		log.Error(string(out))
		return err
	}

	return nil
}

func Killnetem(PID int) error {
	if PID == 0 {
		log.Error(fmt.Sprintf("[tc] Invalid PID=%d", PID))
		return errors.New("Target PID cannot be zero")
	}

	tc := fmt.Sprintf("sudo nsenter -t %d -n tc qdisc delete dev eth0 root", PID)
	cmd := exec.Command("/bin/bash", "-c", tc)
	out, err := cmd.CombinedOutput()
	log.Info(cmd.String())

	if err != nil {
		log.Error(string(out))
		return err
	}

	return nil
}

// float is complicated ¯\_(ツ)_/¯ it can't be compared to exact numbers due to
// variations in precision
func almostZero(f float64) bool {
	return f < 0.0000001
}
