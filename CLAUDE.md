# CLAUDE.md

This file defines the engineering guidelines, architecture, coding standards and development workflow for this repository.

Claude should always read this document before making any modifications.

---

# Project

Name

Cache Stampede Lab

Purpose

This repository is a production-style backend engineering project built to reproduce, analyse and solve a Cache Stampede (Thundering Herd) problem in a Cache-Aside architecture.

The project is inspired by public engineering discussions around distributed caching systems such as Netflix EVCache, while demonstrating general distributed systems concepts rather than recreating any proprietary implementation.

The objective is to build a realistic backend service that evolves through multiple engineering milestones.

This is NOT a tutorial project.

It should resemble a backend service maintained by a platform engineering team.

---

# Primary Goals

The repository should demonstrate

- Production backend architecture
- Distributed caching
- Cache-aside pattern
- Cache stampede reproduction
- Request coalescing
- Distributed locking
- Performance engineering
- Load testing
- Observability
- Benchmarking
- Production debugging

---

# Tech Stack

Language

Go (latest stable version available)

HTTP

Chi Router

Database

PostgreSQL

Cache

Redis

Logging

Uber Zap

Metrics

Prometheus

Dashboards

Grafana

Database Driver

pgx/v5

Redis Client

go-redis/v9

Load Testing

k6

Containers

Docker Compose

Future

OpenTelemetry

Jaeger

---

# Architecture

Always maintain this dependency flow.

HTTP

↓

Handler

↓

Service

↓

Repository

↓

Infrastructure

Infrastructure includes

- PostgreSQL
- Redis

Handlers must never directly communicate with Redis or PostgreSQL.

Business logic belongs inside services.

Repositories should only communicate with storage.

Infrastructure packages should never know about HTTP.

---

# Folder Structure

cmd/

    api/

internal/

    cache/

    config/

    db/

    handler/

    logger/

    metrics/

    middleware/

    model/

    repository/

    service/

migrations/

load/

docs/

scripts/

docker/

---

# Development Philosophy

This project is developed incrementally.

Claude should only implement the milestone requested.

Claude should never anticipate future milestones.

Claude should never implement additional features that have not been requested.

Claude should never rewrite unrelated code.

Large refactors are prohibited unless explicitly requested.

---

# Milestones

Milestone 1

Application foundation

- Config
- Logger
- PostgreSQL connection
- Redis connection
- Health endpoints
- Docker
- Graceful shutdown

Milestone 2

Persistence

- SQL migrations
- Seed data
- Repository layer
- Product model

Milestone 3

Naive Cache Aside

Redis

↓

Miss

↓

Database

↓

Redis SET

↓

Return

Milestone 4

Reproduce Cache Stampede

- k6
- Concurrent requests
- TTL expiration
- Benchmark

Milestone 5

Observability

- Prometheus
- Grafana
- Dashboards

Milestone 6

Redis Distributed Lock

Milestone 7

Go singleflight

Milestone 8

TTL Jitter

Milestone 9

Stale While Revalidate

Milestone 10

Benchmark comparison

---

# Coding Standards

Always

- Follow idiomatic Go
- Prefer composition
- Dependency Injection
- Explicit error handling
- Context propagation
- Constructor functions
- Small packages
- Small functions
- Descriptive naming
- Structured logging

Never

- Global variables
- Panic for recoverable errors
- Circular dependencies
- Hidden side effects
- Tight coupling
- Business logic inside handlers
- SQL inside handlers
- Redis logic inside handlers

---

# Error Handling

Return descriptive errors.

Wrap errors where appropriate.

Never ignore returned errors.

Never log and return the same error multiple times.

---

# Logging

Use structured logging.

Every request should eventually log

- request id
- method
- path
- latency
- status
- cache hit
- cache miss

---

# Performance

Optimise only when the milestone requires it.

Do NOT prematurely optimise.

---

# Documentation

Every exported type should have GoDoc.

Every exported function should have GoDoc.

README should evolve after every milestone.

---

# Testing

All packages should be testable.

Avoid global state.

Dependency Injection should make mocking possible.

---

# Claude Behaviour

Before implementing code

- Explain the design.

After implementing

- Explain what changed.
- Explain why.
- Explain trade-offs.
- Explain how to run.
- Explain how to verify.
- Suggest a commit message.

Then stop.

Never continue into the next milestone until instructed.

---

# LinkedIn Goal

This repository is intended to become an engineering case study.

Every milestone should produce measurable evidence.

Whenever appropriate, Claude should suggest

- benchmark screenshots
- terminal output worth capturing
- Grafana dashboards
- Prometheus metrics
- architecture diagrams
- benchmark tables

These artefacts will later be used to create a technical LinkedIn post documenting the engineering journey from reproducing the failure to implementing production-grade mitigations.