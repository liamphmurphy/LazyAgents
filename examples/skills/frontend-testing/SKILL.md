---
name: frontend-testing
description: "Use when adding or changing UI behaviour and you need to write component, integration or end-to-end tests (Testing Library, Vitest/Jest, Playwright), or when frontend tests are flaky."
---

# Frontend testing

- **Test what users do and see**, not implementation details. Query by role/label/text (`getByRole('button', {name: /save/i})`), not by class names or test ids unless there is no accessible handle. If a role query is hard, the markup probably has an accessibility problem; fix the component.
- **Use `userEvent`** (not `fireEvent`) for interactions; `await` async UI with `findBy*`/`waitFor` (never fixed sleeps). Assert on visible outcomes: text, aria state, navigation, network call made with the right payload.
- **Pyramid:** many fast unit tests for pure logic (formatters, reducers, validators), component tests for states (loading/empty/error/success, form validation, keyboard behaviour), and a *few* end-to-end tests (Playwright) for critical flows (sign-in, create/edit/delete, the main happy path).
- **Network:** mock at the network boundary (MSW) with realistic responses *including errors and slow responses*, typed from the generated API types. Don't mock your own components or hooks.
- **Determinism:** control time (`vi.useFakeTimers`), randomness and viewport; reset state between tests; no dependence on test order or on real external services. In Playwright use auto-waiting locators, isolate data per test, and capture traces on failure.
- **Accessibility in tests:** run `axe` in component/e2e tests for new screens and assert no violations.
- **Visual changes:** run the app and check the result at mobile and desktop widths; attach screenshots to the PR (and snapshot tests only for small, stable components: big DOM snapshots are noise).
- **Before finishing:** run typecheck, lint and the relevant test suites; log the result with `log_event` kind `test_run`. If you changed behaviour and no test failed, you probably need a new test.
