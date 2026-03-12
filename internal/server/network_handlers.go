package server

import (
	"fmt"
	"net/http"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/network"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

// handleListPorts returns the exposed port mappings for a sandbox by
// inspecting its Docker container.
func handleListPorts(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "sandbox name is required")
			return
		}

		sb, err := ctrl.Get(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		if sb.Status.RuntimeID == "" {
			jsonResponse(w, http.StatusOK, []network.ForwardedPort{})
			return
		}

		info, err := ctrl.Runtime().Info(r.Context(), sb.Status.RuntimeID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to inspect container: %v", err))
			return
		}

		ports := make([]network.ForwardedPort, 0, len(info.Ports))
		for _, p := range info.Ports {
			proto := p.Protocol
			if proto == "" {
				proto = "tcp"
			}
			ports = append(ports, network.ForwardedPort{
				SandboxName:   name,
				ContainerPort: p.ContainerPort,
				HostPort:      p.HostPort,
				Protocol:      proto,
			})
		}

		jsonResponse(w, http.StatusOK, ports)
	}
}

// handleListMatrixServices returns service discovery entries for all members
// of a matrix, built from their sandbox IPs and metadata.
func handleListMatrixServices(ctrl *controller.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			errorResponse(w, http.StatusBadRequest, "matrix name is required")
			return
		}

		mx, err := ctrl.GetMatrix(name)
		if err != nil {
			errorResponse(w, http.StatusNotFound, err.Error())
			return
		}

		services := make([]network.ServiceEntry, 0, len(mx.Members))
		for _, member := range mx.Members {
			sandboxName := name + "-" + member.Name
			sb, err := ctrl.Get(sandboxName)
			if err != nil {
				continue
			}

			var ip string
			if sb.Status.RuntimeID != "" {
				info, err := ctrl.Runtime().Info(r.Context(), sb.Status.RuntimeID)
				if err == nil {
					ip = info.IP
				}
			}

			services = append(services, network.ServiceEntry{
				Name:     member.Name,
				Hostname: member.Name + "." + name + ".local",
				IP:       ip,
				Matrix:   name,
			})
		}

		jsonResponse(w, http.StatusOK, services)
	}
}

// portMappingsToForwardedPorts converts runtime port mappings to ForwardedPort
// entries for a given sandbox name.
func portMappingsToForwardedPorts(name string, ports []runtime.PortMapping) []network.ForwardedPort {
	result := make([]network.ForwardedPort, 0, len(ports))
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		result = append(result, network.ForwardedPort{
			SandboxName:   name,
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			Protocol:      proto,
		})
	}
	return result
}
