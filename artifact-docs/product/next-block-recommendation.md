# Product Requirement Document: Closing the Self-Improvement Loop

**Status:** Proposed  
**Author:** Product Manager  
**Strategic Alignment:** AI and Cloud Native (Self-governance)  

## 1. Problem Statement

The secondorder organization aims to be a "zero-human company" that improves recursively. Currently, this loop is broken: the Auditor agent successfully identifies systemic failure patterns and produces archetype patches, but it is unable to submit or apply them because:
1. The `SO_API_KEY` is missing from the Auditor's execution environment.
2. The manual review/application flow for archetype patches is not yet fully automated or integrated into the main execution cycle.
3. Pending patches for the CEO, Engineer, QA, and DevOps archetypes have accumulated in documentation but remain unapplied to production.

## 2. Proposed Solution: Closing the Loop

This work block will focus on unblocking the Auditor and establishing a robust, automated path from "Audit Finding" to "System Improvement."

## 3. Scope

- **Auditor Environment Unblocking:** Expose the `SO_API_KEY` to the Auditor agent's environment so it can interact with the Secondorder API (specifically for submitting patches).
- **Archetype Patching Automation:** Finalize and verify the `/api/v1/archetype-patches` endpoint. Ensure that patches proposed by the Auditor can be applied to the `archetypes/` directory (with human or CEO sign-off).
- **Institutional Memory Consolidation:** Apply the pending patches from `decisions/archetype-patches-65ff2dbb.md` which include critical "no-commit" rules, security requirements, and async UI pattern guidance.
- **Audit-to-Issue Integration:** Improve the Auditor's ability to not just produce reports, but to directly create follow-up issues for systemic bugs (e.g., the redirect regression identified in recent audits).

## 4. Rationale

This is the single highest-leverage work block for the platform. While features like the Wiki and VTMS improvements add value, "Closing the Loop" unlocks the organization's ability to learn and fix its own inefficiencies. Every run that occurs while this loop is broken is a missed opportunity for institutional growth.

## 5. Proposed Acceptance Criteria

- [ ] Auditor agent can successfully call the `/api/v1/archetype-patches` endpoint without authorization errors.
- [ ] Pending patches for CEO, Engineer, QA, and DevOps are applied and verified in the `archetypes/` folder.
- [ ] A test audit run successfully identifies a trivial improvement and proposes a patch that can be applied with one click.
- [ ] Documentation for the "Async UI Pattern" is added to the shared knowledge base/archetypes to reduce retry rates for Frontend tasks.
- [ ] `SECONDORDER_API_KEY` is confirmed present in the Auditor's environment variables.

## 6. Alternative Considered: Wiki Phase 2

While Wiki Phase 2 (Search, Inter-linking, Versioning) is a highly desirable feature set that was recently completed at Phase 1, it is a feature enhancement. "Closing the Loop" is an architectural necessity for the project's primary mission of autonomous self-governance. We recommend pursuing the Wiki Phase 2 immediately following this block.
