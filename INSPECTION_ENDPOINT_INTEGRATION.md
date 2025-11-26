# Inspection Endpoint Integration Guide

This document explains how to integrate the inspection endpoint into the Beskar7 controller manager.

## Overview

The inspection endpoint (`controllers/inspection_handler.go`) provides an HTTP API for inspection images to submit hardware reports.

## API Endpoint

**URL:** `POST /api/v1/inspection`

**Request Body:**
```json
{
  "namespace": "default",
  "hostName": "server-01",
  "manufacturer": "Dell Inc.",
  "model": "PowerEdge R750",
  "serialNumber": "ABC123",
  "cpus": [
    {
      "id": "0",
      "vendor": "Intel",
      "model": "Xeon Gold 6254",
      "cores": 18,
      "threads": 36,
      "frequency": "3.1GHz"
    }
  ],
  "memory": [
    {
      "id": "DIMM0",
      "type": "DDR4",
      "capacity": "32GB",
      "speed": "3200MHz"
    }
  ],
  "disks": [
    {
      "name": "/dev/sda",
      "model": "Samsung 870 EVO",
      "sizeGB": 500,
      "type": "SSD",
      "serialNumber": "S5H1NS0T123456"
    }
  ],
  "nics": [
    {
      "name": "eth0",
      "macAddress": "00:25:90:f0:79:00",
      "driver": "ixgbe",
      "speed": "1Gbps",
      "ipAddresses": ["192.168.1.100"]
    }
  ],
  "bootModeDetected": "UEFI",
  "firmwareVersion": "2.15.0"
}
```

**Response (Success):**
```json
{
  "status": "success",
  "message": "Inspection report received and processed"
}
```

**Response (Error):**
```
HTTP 400 Bad Request: Invalid JSON or missing fields
HTTP 404 Not Found: PhysicalHost not found
HTTP 500 Internal Server Error: Failed to update PhysicalHost
```

## Integration Steps

### 1. Add to main.go

Create or update `main.go` (usually in `cmd/manager/` or root):

```go
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	"github.com/wrkode/beskar7/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(infrastructurev1beta1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var inspectionPort int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&inspectionPort, "inspection-port", 8082, "The port for the inspection report API.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "beskar7.infrastructure.cluster.x-k8s.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup controllers
	if err = (&controllers.PhysicalHostReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("PhysicalHost"),
		Recorder: mgr.GetEventRecorderFor("physicalhost-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PhysicalHost")
		os.Exit(1)
	}

	if err = (&controllers.Beskar7MachineReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("Beskar7Machine"),
		Recorder: mgr.GetEventRecorderFor("beskar7machine-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Beskar7Machine")
		os.Exit(1)
	}

	if err = (&controllers.Beskar7ClusterReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("Beskar7Cluster"),
		Recorder: mgr.GetEventRecorderFor("beskar7cluster-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Beskar7Cluster")
		os.Exit(1)
	}

	// Setup inspection server
	if err := controllers.SetupInspectionServer(mgr, inspectionPort); err != nil {
		setupLog.Error(err, "unable to setup inspection server")
		os.Exit(1)
	}
	setupLog.Info("Inspection server configured", "port", inspectionPort)

	// Setup webhooks (if any)
	// if err = (&infrastructurev1beta1.PhysicalHost{}).SetupWebhookWithManager(mgr); err != nil {
	//     setupLog.Error(err, "unable to create webhook", "webhook", "PhysicalHost")
	//     os.Exit(1)
	// }

	// Add health and ready checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
```

### 2. Update Dockerfile (if needed)

Ensure the inspection port is exposed:

```dockerfile
# Expose metrics, health, and inspection ports
EXPOSE 8080 8081 8082
```

### 3. Update Kubernetes Deployment

Update `config/manager/manager.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        image: controller:latest
        args:
        - --leader-elect
        - --inspection-port=8082
        ports:
        - containerPort: 8080
          name: metrics
          protocol: TCP
        - containerPort: 8081
          name: health
          protocol: TCP
        - containerPort: 8082
          name: inspection
          protocol: TCP
```

### 4. Create Service for Inspection Endpoint

Create `config/manager/inspection-service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: beskar7-inspection
  namespace: beskar7-system
spec:
  selector:
    control-plane: beskar7-controller-manager
  ports:
  - name: inspection
    port: 8082
    targetPort: inspection
    protocol: TCP
  type: ClusterIP
```

### 5. Update Kustomization

Add to `config/default/kustomization.yaml`:

```yaml
resources:
- ../manager
- ../manager/inspection-service.yaml
```

## Testing the Inspection Endpoint

### 1. Port Forward (for local testing)

```bash
kubectl port-forward -n beskar7-system deployment/beskar7-controller-manager 8082:8082
```

### 2. Send Test Report

```bash
curl -X POST http://localhost:8082/api/v1/inspection \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "default",
    "hostName": "server-01",
    "manufacturer": "Dell Inc.",
    "model": "PowerEdge R750",
    "serialNumber": "TEST123",
    "cpus": [
      {
        "id": "0",
        "vendor": "Intel",
        "model": "Xeon Gold 6254",
        "cores": 18,
        "threads": 36,
        "frequency": "3.1GHz"
      }
    ],
    "memory": [
      {
        "id": "DIMM0",
        "type": "DDR4",
        "capacity": "32GB",
        "speed": "3200MHz"
      }
    ]
  }'
```

Expected response:
```json
{
  "status": "success",
  "message": "Inspection report received and processed"
}
```

### 3. Verify Report Stored

```bash
kubectl get physicalhost server-01 -o jsonpath='{.status.inspectionReport}' | jq
```

## Inspector Image Integration

The inspection image (Alpine Linux) should:

1. **Boot via iPXE** with kernel parameters:
   ```
   beskar7.api=http://beskar7-inspection.beskar7-system.svc.cluster.local:8082
   beskar7.namespace=default
   beskar7.host=server-01
   ```

2. **Run inspection scripts** to gather hardware info

3. **Submit report** via HTTP POST:
   ```bash
   #!/bin/sh
   # In inspection image
   
   BESKAR7_API="$( cat /proc/cmdline | grep -o 'beskar7.api=[^ ]*' | cut -d= -f2)"
   BESKAR7_NAMESPACE="$(cat /proc/cmdline | grep -o 'beskar7.namespace=[^ ]*' | cut -d= -f2)"
   BESKAR7_HOST="$(cat /proc/cmdline | grep -o 'beskar7.host=[^ ]*' | cut -d= -f2)"
   
   # Gather hardware info (simplified)
   REPORT=$(cat <<EOF
   {
     "namespace": "$BESKAR7_NAMESPACE",
     "hostName": "$BESKAR7_HOST",
     "manufacturer": "$(dmidecode -s system-manufacturer)",
     "model": "$(dmidecode -s system-product-name)",
     "serialNumber": "$(dmidecode -s system-serial-number)",
     "cpus": $(lscpu -J | jq '.lscpu'),
     "memory": $(free -h | awk 'NR==2{print $2}'),
     "disks": $(lsblk -J),
     "nics": $(ip -j addr)
   }
   EOF
   )
   
   # Submit report
   curl -X POST "$BESKAR7_API/api/v1/inspection" \
     -H "Content-Type: application/json" \
     -d "$REPORT"
   
   # Wait for next steps or kexec into target OS
   ```

## Security Considerations

### 1. Authentication (Optional)

Add token-based auth to inspection handler:

```go
func (h *InspectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Verify token
    token := r.Header.Get("Authorization")
    if token != "Bearer "+os.Getenv("INSPECTION_TOKEN") {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    // ... rest of handler
}
```

Pass token via kernel parameters:
```
beskar7.token=secret-token-here
```

### 2. Network Policy

Restrict access to inspection endpoint:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: beskar7-inspection-ingress
  namespace: beskar7-system
spec:
  podSelector:
    matchLabels:
      control-plane: beskar7-controller-manager
  policyTypes:
  - Ingress
  ingress:
  - ports:
    - protocol: TCP
      port: 8082
    from:
    - podSelector: {}  # Allow from same namespace
    # Or specific IP ranges for provisioning network
```

### 3. Rate Limiting

Add rate limiting to prevent abuse:

```go
import "golang.org/x/time/rate"

var limiter = rate.NewLimiter(10, 20)  // 10 req/s, burst of 20

func (h *InspectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if !limiter.Allow() {
        http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
        return
    }
    // ... rest of handler
}
```

## Troubleshooting

### Inspection reports not received

**Check:**
1. Inspection server is running:
   ```bash
   kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep inspection-server
   ```
2. Port is accessible:
   ```bash
   kubectl get svc -n beskar7-system beskar7-inspection
   ```
3. Network connectivity from inspection image
4. Kernel parameters passed correctly

### Reports received but not updating PhysicalHost

**Check:**
1. Controller logs for errors:
   ```bash
   kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep inspection-handler
   ```
2. PhysicalHost exists:
   ```bash
   kubectl get physicalhost <name> -n <namespace>
   ```
3. Report JSON is valid

## Next Steps

After integrating the inspection endpoint:

1. Build beskar7-inspector Alpine image
2. Test end-to-end workflow
3. Add monitoring and metrics
4. Implement kexec for final OS boot

---

**Status:** Ready for integration ✅  
**File:** `controllers/inspection_handler.go`  
**Port:** 8082 (configurable)

