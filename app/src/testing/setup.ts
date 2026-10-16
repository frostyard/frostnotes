// Vitest setup for component tests (happy-dom, Testing Library). Globals are
// off, so cleanup is registered here.
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Testing Library prints the DOM with a failed query; keep it short enough
// that the failure itself stays in view.
process.env.DEBUG_PRINT_LIMIT ??= "1500";

configure({ asyncUtilTimeout: 5000 });

afterEach(cleanup);
