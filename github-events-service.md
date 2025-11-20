# GitHub Events Service Architecture

```mermaid
flowchart LR
    subgraph GitHub Cloud
        GH[GitHub Webhooks]
    end

    subgraph Internet
        DNS[DNS: github-events.cncf.io -> Reserved IP]
    end

    subgraph "Oracle Cloud (OCI)"
        subgraph Cluster Network
            LBService[Ingress Controller Service\n(type=LoadBalancer, loadBalancerIP=Reserved IP)]
            Ingress[Ingress-NGINX Controller]
            Service[maintainerd Service\n(ClusterIP)]
            Pod[maintainerd Pod]
        end
        subgraph MetalLB System
            Pool[IPAddress Pool\ncontains Reserved IP]
            Speakers[Speaker Pods\nannounce IP via BGP/ARP]
        end
    end

    GH -->|HTTPS webhook| DNS --> LBService
    LBService -->|requests IP| Pool
    Speakers -->|advertise IP| Internet
    LBService --> Ingress --> Service --> Pod
```

## Flow Summary
- GitHub delivers webhook events to `github-events.cncf.io`.
- DNS resolves the hostname to the reserved OCI public IP.
- The ingress controller Service is configured with `loadBalancerIP=<reserved-ip>`, so MetalLB binds that address.
- MetalLB speakers advertise the IP; traffic enters the cluster via the ingress controller.
- The ingress routes traffic to the internal `maintainerd` Service (ClusterIP) and on to the pod.

