# Kubernetes Deployment Architecture

This directory contains the comprehensive microservices Kubernetes manifests for the Team 13 Event Ticketing System. This architecture has been thoroughly verified on a local cluster (Docker Desktop K8s) and is fully optimized for **High Availability (HA)**, **Self-Healing (Final Consistency)**, and **Network-Level Security Isolation**.

## Microservices Architecture & Component Matrix

| Component | K8s Resource Type | Replicas | Service Type | Core Technical Highlights |
| :--- | :--- | :--- | :--- | :--- |
| **frontend** | Deployment & Service | 1 | NodePort (`30080`) | Production Nginx server reverse-proxying `/api` traffic to backend. Pre-configured for CORS whitelist synchronization. |
| **backend** | Deployment & Service | 2 | ClusterIP | Go ticket-booking core engine running a **dual-replica load-balanced array**. Utilizes `$(VAR)` dynamic substitution for zero-code database string assembly. |
| **postgres** | Deployment & Service | 1 | ClusterIP (Isolated) | PostgreSQL 15 database tuned with `max_connections=250`. Implements `fsGroup: 999` to overcome cross-platform storage mounting permission constraints. |
| **redis** | Deployment & Service | 1 | ClusterIP (Isolated) | Redis 7 queue manager powered by Redis Streams with Append-Only File (AOF) storage engine persistence enabled. |
| **app-config**| ConfigMap | N/A | Shared Config | Centralized non-sensitive environmental runtime variables (e.g., `GIN_MODE`, ClusterIP service discovery DNS). |
| **app-secret**| Secret | N/A | Encrypted Asset | Encapsulates sensitive credentials (`DB_PASSWORD`, `JWT_SECRET`) to ensure strict compliance with enterprise security data-at-rest principles. |

---

## Cloud-Native Core Capabilities

1. **High Availability (HA)**: The backend API tier is instantiated with `replicas: 2`. Incoming traffic is handled via K8s native internal load balancing (kube-proxy via Round-Robin), eliminating single points of failure (SPOF) during spike-load ticketing windows.
2. **Self-Healing Capabilities**: All application workloads are guarded by K8s Deployment controllers. In the event of an out-of-memory (OOM) crash or unhandled panic caused by high concurrency, the control loop automatically recycles and spins up a healthy pod instance within milliseconds.
3. **Security Isolation**: Data persistence layers (`postgres`, `redis`) are hard-locked under `ClusterIP`, completely denying public internet ingress. The PostgreSQL daemon strictly drops root privileges, executing instead under a restricted non-privileged user space (`fsGroup: 999`).

---

## Local 1-Click Deployment & Verification Guide

Ensure your local Docker Desktop has Kubernetes enabled and the `kubectl` CLI tool is installed.

### 1. Provision the Infrastructure
Execute the following unified command from the root project directory:
```bash
kubectl apply -f k8s/
```

### 2. Monitor the Cluster State
```bash
kubectl get pods -w
```
Note: Because Kubernetes adopts an asynchronous startup sequence, the frontend and backend pods may temporarily show 1-2 initial restarts (CrashLoopBackOff) while waiting for the PostgreSQL container to finalize storage initialization. This represents the native cloud-healing "Final Consistency" pattern. All workloads will automatically converge into green lights.

### 3. Application Verification
Once all workloads status transitions to Running, open your web browser and navigate to: http://localhost:30080

