Below is a **Skill Definition (skill.md)** that teaches an AI assistant to apply **both SOLID principles** and **Modular Monolith Architecture** as complementary design philosophies. It merges the two into a single consistent set of rules, checklists, and examples.

```markdown
# SOLID + Modular Monolith Architecture (Combined Skill)

## Purpose

Always design and modify code using:

- **SOLID principles** for clean, maintainable class/function design  
- **Modular Monolith Architecture** for clear module boundaries, ownership, and cross-module contracts  

The system remains a single deployable application while preserving strong structural integrity so that parts can evolve independently with high cohesion and low coupling.

## When to use this skill

- New feature design  
- Refactoring or bug fixes that touch architecture  
- Folder/package structure decisions  
- Service/domain/repository/controller design  
- API/interface design  
- Cross-module communication  
- Business logic placement  
- Dependency cleanup  
- Code review  

---

## Core Principles (SOLID + Modular)

### 1. Single Responsibility (S) → Module Ownership

- Every **class** should have one reason to change.  
- Every **module** should own one business concept (domain logic, data access, internal models, public contracts, invariants, business rules).  
- Do not place business logic in shared utilities, controllers, routes, or infrastructure layers.

**Bad** (violates both):
```txt
shared/
  user-utils.ts
  order-utils.ts
  payment-rules.ts
```

**Good**:
```txt
modules/
  users/   (owning everything about users)
  orders/  (owning everything about orders)
  payments/
```

### 2. Open/Closed (O) → Contracts over Direct Coupling

- Classes/modules should be **open for extension, closed for modification**.  
- Modules communicate through **explicit contracts** (interfaces, DTOs, events, commands, queries, facades).  
- Do not import another module’s internal domain models, repositories, or database tables directly.

**Bad**:
```ts
import { UserRepository } from "../users/infrastructure/UserRepository";
```

**Better**:
```ts
import { UserReaderContract } from "../users/contracts/UserReaderContract";
```

### 3. Liskov Substitution (L) → Ports & Adapters

- Subtypes (implementations) must be replaceable without breaking the system.  
- Use **ports** (interfaces) for dependencies that cross boundaries; **adapters** (infrastructure) implement them.  
- Any implementation of a given contract must satisfy the contract’s expectations.

### 4. Interface Segregation (I) → Small, Focused Contracts

- Clients should not be forced to depend on interfaces they do not use.  
- Each module’s `contracts/` folder should expose **only what other modules need** – small, purpose-specific interfaces/DTOs.  
- Avoid one “god” contract per module.

### 5. Dependency Inversion (D) → Depend on Abstractions

- High-level modules should not depend on low-level modules; both should depend on abstractions.  
- Modules depend on **interfaces/contracts**, not on concrete implementations from other modules.  
- Infrastructure implements the abstractions defined by the domain/application layer.

---

## Architecture Rules (Merged)

### Rule 1: No cross-module internal imports

Allowed:
```ts
import { UserReader } from "@/modules/users/contracts";
```
Not allowed:
```ts
import { UserEntity } from "@/modules/users/domain/UserEntity";
import { UserRepository } from "@/modules/users/infrastructure/UserRepository";
```

### Rule 2: Domain logic belongs in the owning module

Business decisions live in the **domain** folder of the module that owns the concept.  
Application services orchestrate use cases; infrastructure provides adapters.

### Rule 3: Use application services for use cases

Expose public behaviour through application (use-case) services – not through controllers or repositories directly.

```txt
orders/application/CreateOrderUseCase.ts
orders/application/CancelOrderUseCase.ts
```

### Rule 4: Shared code must be truly shared

`shared/` folders contain only domain‑neutral utilities (e.g., string formatting, date helpers).  
Business rules are never placed in shared code.

### Rule 5: Prefer events for decoupled side effects

When one module needs to notify others (e.g., “order placed” → “send email”, “update inventory”), use **domain events** or **integration events**.

### Rule 6: Database ownership

Each module owns its persistence. Other modules query via contracts – never via direct SQL or ORM access across modules.

---

## TDD + Combined Skill

### Red (Write failing test)

- Clarify which **module** owns the behaviour  
- Identify the **contract** or **use case** under test  
- Input / expected output / side effect  
- Business rule that must be protected  

### Green (Implement smallest code)

Write only enough to pass the test, respecting SOLID and module boundaries.

### Refactor (Improve design)

- Move misplaced logic into the correct module  
- Extract interfaces for cross-module dependencies  
- Ensure no circular dependencies  

### Testing rules

- **Domain tests**: fast, isolated, no database/HTTP/framework  
- **Application tests**: test use cases (not controllers)  
- **Contract tests**: verify cross-module public contracts, not private implementations  
- **Integration tests**: only where truly necessary (e.g., to verify event publishing works)

---

## Checklists

### Pre‑code checklist (before writing any code)

- [ ] Which module owns this behaviour?  
- [ ] Is this domain logic, application logic, or infrastructure logic?  
- [ ] Does this require a new **contract** (interface/DTO/event/command/query)?  
- [ ] Am I importing another module’s **internals**?  
- [ ] Should this be an **event** instead of a direct call?  
- [ ] Is this code truly **shared** or does it belong to a specific module?  
- [ ] Can this module be tested **independently**?  
- [ ] Does this design violate **SOLID**? Which principle?

### Review checklist (code review)

**Flag these**:
- [ ] Another module’s internal files are imported  
- [ ] Business logic lives **outside** the owning module (e.g., in a shared utility or controller)  
- [ ] Shared code contains domain‑specific rules  
- [ ] Contracts are missing, too broad, or leak implementation details  
- [ ] Circular dependencies exist  
- [ ] Infrastructure leaks into domain logic (e.g., using ORM-specific annotations in domain entities)  
- [ ] A class/module has more than one clear reason to change (violates SRP)  
- [ ] Dependency injection is used incorrectly (high-level depends on low-level concrete class)  
- [ ] Interface segregation is violated (fat interfaces)

---

## Example of a Well-Structured Module

```txt
modules/
  orders/
    domain/
      Order.ts          (entities, value objects, domain events)
      OrderRepository.ts (port/interface)
      OrderService.ts   (domain logic, if needed)
    application/
      CreateOrderUseCase.ts   (orchestrates domain + infrastructure)
      CancelOrderUseCase.ts
    infrastructure/
      OrderRepositoryPostgres.ts (adapter implementing port)
      OrderController.ts         (HTTP adapter)
    contracts/
      IOrderReader.ts     (interface for read queries)
      OrderCreatedEvent.ts (integration event DTO)
      PlaceOrderDto.ts    (input DTO for use case)
    index.ts              (re‑exports only contracts/ and optionally use case constructors)
```

**Cross‑module communication example** (using contracts):

```ts
// Inside modules/billing/application/OnOrderPlacedHandler.ts
import { IOrderReader } from "@/modules/orders/contracts/IOrderReader";

export class OnOrderPlacedHandler {
  constructor(private orderReader: IOrderReader) {}

  async handle(event: { orderId: string }) {
    const order = await this.orderReader.getById(event.orderId);
    // ...create invoice using order data...
  }
}
```

---

## Summary

This skill merges **SOLID** (class/function level) with **Modular Monolith Architecture** (module/boundary level) into one coherent design philosophy. Use it to:

- Keep classes small and focused (SRP)  
- Keep modules independent and contract‑based (OCP, ISP, DIP)  
- Ensure substitutability of adapters (LSP)  
- Prevent architectural decay in growing monoliths  

When in doubt, ask: **“Does this decision respect both SOLID and module boundaries?”**
```

This `skill.md` can be used as a definitive guide for an AI assistant to consistently apply both sets of principles. It clarifies that SOLID works at the micro-level (classes, functions) while the modular monolith rules work at the macro-level (modules, boundaries), and they reinforce each other.
