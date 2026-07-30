# Aegis: Distributed Network Access Control for Academic Laboratories

## Overview
Aegis is a centralized management and distributed network filtering system designed to enforce dynamic internet access policies across computer laboratory environments during academic evaluations. Developed as part of an undergraduate capstone project (TCC) at the Federal University of Campina Grande (UFCG), Aegis addresses the scalability, maintainability, and single-point-of-failure (SPOF) limitations inherent in traditional centralized proxy solutions.

## Key Features
- **Multi-Layer Traffic Filtering:** Enforces access rules across domain levels (L7) and IP/port/protocol levels (L4).
- **Logical Group Management:** Enables administrators to group laboratory machines and apply distinct access policies per room or examination context.
- **Real-Time Policy Propagation:** Pushes rule updates instantly to target endpoints without requiring service restarts or interrupting active user sessions.
- **Autonomous Endpoint Operation (Failover):** Local agents maintain active network filtering using cached policies or fallback defaults if the central server becomes unreachable.
- **Low Resource Footprint:** Designed for optimal performance on modest client hardware within ephemeral operating system environments.
- **Audit Logging & Real-Time Monitoring:** Tracks administrative operations and provides real-time visibility into endpoint connectivity and filtering status.

## System Architecture Overview
The system decouples policy administration from packet processing by employing a two-tiered architecture:

| Component | Description & Responsibilities |
| :--- | :--- |
| **Orchestrator** | Central control plane responsible for rule storage, policy definition, logical group management, and real-time distribution of configurations via API. |
| **Sentinel** | Lightweight daemon running locally on each endpoint. Receives policies from the Orchestrator, configures local kernel network filters, and enforces traffic constraints autonomously. |

## Technical Requirements & Constraints
- **Target OS:** Linux-based distributions (optimized for ephemeral live desktop images).
- **Client Storage Footprint:** Maximum local installation footprint under 100 MB.
- **Client Resource Limits:** Runtime RAM usage limited to under 100 MB; CPU utilization capped below 5%.
- **Propagation Latency:** Policy distribution completes within 5 seconds across group endpoints.
- **Filtering Overhead:** Local request processing adds no more than 100 ms of average network latency.

## Academic Context
This software is developed as an undergraduate capstone project (Trabalho de Conclusão de Curso - TCC) under the Guardians UFCG initiative.