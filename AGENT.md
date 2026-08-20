# Skill: Rigorous AST-Guided Code Engineering

You are a high-precision coding agent. You MUST NOT make raw text-based assumptions, regex-only modifications, or partial refactors. All code discovery, structural analysis, and edits must strictly leverage Abstract Syntax Tree (AST) tools to guarantee zero breaking changes across the repository.

---

**Mandatory AST Execution Protocol**

* **Phase 1: AST Discovery & Mapping**
* **Map Symbol Call-Graphs:** Query the repository AST (via `ast-grep`, `tree-sitter`, or LSP indexers) to identify every definition, type signature, and downstream import reference across all files.
* **Resolve Context Boundaries:** Use AST scope nodes (`FunctionDeclaration`, `ClassDeclaration`, `ModuleBlock`) to isolate variables—do not rely on string matching.


* **Phase 2: Targeted Structural Edits**
* **Atomic Signature Updates:** If modifying a function signature or class interface, trace the exact AST reference graph. Update all callers across the repository within the same operation.
* **Node-Level Precision:** Perform modifications targeting specific AST node types (e.g., `CallExpression`, `JSXElement`). Never manipulate raw text lines where edits could accidently target comments, docstrings, or string literals.


* **Phase 3: AST Verification**
* **Syntax Validation:** Re-parse all modified files into structural ASTs immediately after editing to verify valid syntax.
* **Dependency Graph Audit:** Re-run cross-file AST symbol queries to confirm no dangling references, broken exports, or unhandled type mismatches remain.



---

| Operation | Prohibited Method | Mandatory AST Method |
| --- | --- | --- |
| **Symbol Search** | grep -r "myFunction" | ast-grep --pattern 'myFunction($$$)' |
| **Refactoring** | String Replace / Regex | AST Node Mutation / Structural Rewrite |
| **Impact Analysis** | File-by-file text review | Repository Call Graph & AST Dependency Mapping |
| **Syntax Check** | Visual inspection | Full AST Parser Validation |

---

**Non-Negotiable Guardrails**

* **No Unverified Edits:** Never modify a single file without first checking its global import/export linkages using AST graph parsing.
* **Fail-Fast Policy:** If an AST tool returns ambiguous syntax trees or unparseable blocks, STOP execution immediately and report the structural error.
* **Comment & String Safety:** Never alter AST nodes classified as Comment or StringLiteral during structural refactoring unless explicitly instructed to do so.