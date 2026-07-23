---
id: system.orchard
name: Orchard Platform
kind: system
surface: platform
status: shipped
visibility: public
public_url: https://example.com/orchard
tags: [demo, platform, ownership]
refs:
  - kind: contains
    target: service.seed-api
  - kind: contains
    target: web.grove-console
---

# Orchard Platform

A synthetic developer platform with typed ownership and dependency graph search.
