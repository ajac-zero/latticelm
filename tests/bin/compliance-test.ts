import {
  testTemplates,
  runAllTests,
  type TestConfig,
  type TestResult,
} from "../src/compliance-tests.ts";

const colors = {
  green: (s: string) => `\x1b[32m${s}\x1b[0m`,
  red: (s: string) => `\x1b[31m${s}\x1b[0m`,
  yellow: (s: string) => `\x1b[33m${s}\x1b[0m`,
  gray: (s: string) => `\x1b[90m${s}\x1b[0m`,
};

interface CliArgs {
  baseUrl?: string;
  apiKey?: string;
  model?: string;
  authHeader?: string;
  noBearer?: boolean;
  noAuth?: boolean;
  filter?: string[];
  verbose?: boolean;
  json?: boolean;
  help?: boolean;
}

function parseArgs(argv: string[]): CliArgs {
  const args: CliArgs = {};
  let i = 0;

  while (i < argv.length) {
    const arg = argv[i];
    const nextArg = argv[i + 1];

    switch (arg) {
      case "--base-url":
      case "-u":
        args.baseUrl = nextArg;
        i += 2;
        break;
      case "--api-key":
      case "-k":
        args.apiKey = nextArg;
        i += 2;
        break;
      case "--model":
      case "-m":
        args.model = nextArg;
        i += 2;
        break;
      case "--auth-header":
        args.authHeader = nextArg;
        i += 2;
        break;
      case "--no-bearer":
        args.noBearer = true;
        i += 1;
        break;
      case "--no-auth":
        args.noAuth = true;
        i += 1;
        break;
      case "--filter":
      case "-f":
        args.filter = nextArg.split(",").map((s) => s.trim());
        i += 2;
        break;
      case "--verbose":
      case "-v":
        args.verbose = true;
        i += 1;
        break;
      case "--json":
        args.json = true;
        i += 1;
        break;
      case "--help":
      case "-h":
        args.help = true;
        i += 1;
        break;
      default:
        i += 1;
    }
  }

  return args;
}

function printHelp() {
  console.log(`
Usage: npm run test:compliance -- [options]

Options:
  -u, --base-url <url>        Gateway base URL (default: http://localhost:8080)
  -k, --api-key <key>         API key (or set OPENRESPONSES_API_KEY env var)
      --no-auth               Skip authentication header entirely
  -m, --model <model>         Model name (default: gpt-4o-mini)
      --auth-header <name>    Auth header name (default: Authorization)
      --no-bearer             Disable Bearer prefix in auth header
  -f, --filter <ids>          Filter tests by ID (comma-separated)
  -v, --verbose               Verbose output with request/response details
      --json                  Output results as JSON
  -h, --help                  Show this help message

Test IDs:
  ${testTemplates.map((t) => t.id).join(", ")}

Examples:
  npm run test:compliance
  npm run test:compliance -- --model claude-3-5-sonnet-20241022
  npm run test:compliance -- --filter basic-response,streaming-response
  npm run test:compliance -- --verbose --filter basic-response
  npm run test:compliance -- --json > results.json
`);
}

function getStatusIcon(status: TestResult["status"]): string {
  switch (status) {
    case "passed":
      return colors.green("✓");
    case "failed":
      return colors.red("✗");
    case "running":
      return colors.yellow("◉");
    case "pending":
      return colors.gray("○");
  }
}

function printResult(result: TestResult, verbose: boolean) {
  const icon = getStatusIcon(result.status);
  const duration = result.duration ? ` (${result.duration}ms)` : "";
  const events =
    result.streamEvents !== undefined ? ` [${result.streamEvents} events]` : "";
  const name =
    result.status === "failed" ? colors.red(result.name) : result.name;

  console.log(`${icon} ${name}${duration}${events}`);

  if (result.status === "failed" && result.errors?.length) {
    for (const error of result.errors) {
      console.log(`  ${colors.red("✗")} ${error}`);
    }

    if (verbose) {
      if (result.request) {
        console.log(`\n  Request:`);
        console.log(
          `  ${JSON.stringify(result.request, null, 2).split("\n").join("\n  ")}`,
        );
      }
      if (result.response) {
        console.log(`\n  Response:`);
        const responseStr =
          typeof result.response === "string"
            ? result.response
            : JSON.stringify(result.response, null, 2);
        console.log(`  ${responseStr.split("\n").join("\n  ")}`);
      }
    }
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));

  if (args.help) {
    printHelp();
    process.exit(0);
  }

  const baseUrl = args.baseUrl || "http://localhost:8080";
  const apiKey = args.apiKey || process.env.OPENRESPONSES_API_KEY || "";

  if (!apiKey && !args.noAuth) {
    // No auth is fine for local gateway without auth enabled
  }

  const config: TestConfig = {
    baseUrl,
    apiKey,
    model: args.model || "gpt-4o-mini",
    authHeaderName: args.authHeader || "Authorization",
    useBearerPrefix: !args.noBearer,
  };

  if (args.filter?.length) {
    const availableIds = testTemplates.map((t) => t.id);
    const invalidFilters = args.filter.filter(
      (id) => !availableIds.includes(id),
    );
    if (invalidFilters.length) {
      console.error(
        `${colors.red("Error:")} Invalid test IDs: ${invalidFilters.join(", ")}`,
      );
      console.error(`Available test IDs: ${availableIds.join(", ")}`);
      process.exit(1);
    }
  }

  const allUpdates: TestResult[] = [];

  const onProgress = (result: TestResult) => {
    if (args.filter && !args.filter.includes(result.id)) {
      return;
    }
    allUpdates.push(result);
    if (!args.json && result.status !== "running") {
      printResult(result, args.verbose || false);
    }
  };

  if (!args.json) {
    console.log(`Running compliance tests against: ${baseUrl}`);
    console.log(`Model: ${config.model}`);
    if (args.filter) {
      console.log(`Filter: ${args.filter.join(", ")}`);
    }
    console.log();
  }

  await runAllTests(config, onProgress);

  const finalResults = allUpdates.filter(
    (r) => r.status === "passed" || r.status === "failed",
  );
  const passed = finalResults.filter((r) => r.status === "passed").length;
  const failed = finalResults.filter((r) => r.status === "failed").length;

  if (args.json) {
    console.log(
      JSON.stringify(
        {
          summary: { passed, failed, total: finalResults.length },
          results: finalResults,
        },
        null,
        2,
      ),
    );
  } else {
    console.log(`\n${"=".repeat(50)}`);
    console.log(
      `Results: ${colors.green(`${passed} passed`)}, ${colors.red(`${failed} failed`)}, ${finalResults.length} total`,
    );

    if (failed > 0) {
      console.log(`\nFailed tests:`);
      for (const r of finalResults) {
        if (r.status === "failed") {
          console.log(`\n${r.name}:`);
          for (const e of r.errors || []) {
            console.log(`  - ${e}`);
          }
        }
      }
    } else {
      console.log(`\n${colors.green("✓ All tests passed!")}`);
    }
  }

  process.exit(failed > 0 ? 1 : 0);
}

main().catch((error) => {
  console.error(colors.red("Fatal error:"), error);
  process.exit(1);
});
