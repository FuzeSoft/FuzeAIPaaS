package api

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

func ssrfGuard(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid connection target: %q", rawURL)
	}
	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := assertSafeIP(ip); err != nil {
			return err
		}
		return nil
	}
	
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %v", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("host %q resolves to no address", host)
	}
	for _, ip := range ips {
		if err := assertSafeIP(ip); err != nil {
			return err
		}
	}
	return nil
}

func assertSafeIP(ip net.IP) error {
	if !ip.IsGlobalUnicast() {
		return fmt.Errorf("refusing connection to non-global-unicast address %s", ip)
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("refusing connection to loopback/link-local address %s", ip)
	}
	if ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("refusing connection to private/unspecified address %s", ip)
	}
	return nil
}

func kubeconfigServerURLs(kubeconfig string) ([]string, error) {
	cfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %v", err)
	}
	var urls []string
	for _, c := range cfg.Clusters {
		if c != nil && c.Server != "" {
			urls = append(urls, c.Server)
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("kubeconfig contains no cluster server")
	}
	return urls, nil
}

func guardKubeconfigServers(kubeconfig string) error {
	urls, err := kubeconfigServerURLs(kubeconfig)
	if err != nil {
		return err
	}
	for _, u := range urls {
		if err := ssrfGuard(u); err != nil {
			return err
		}
	}
	return nil
}

func ssrfDialProbe(rawURL string, timeout time.Duration) error {
	if err := ssrfGuard(rawURL); err != nil {
		return err
	}
	u, _ := url.Parse(rawURL)
	host := u.Host
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = net.JoinHostPort(h, p)
	} else {
		host = net.JoinHostPort(host, "443")
	}
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return fmt.Errorf("connection probe failed: %v", err)
	}
	_ = conn.Close()
	return nil
}