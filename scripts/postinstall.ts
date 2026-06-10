// @ts-nocheck
// Runs via bun runtime, not tsc

// @ts-expect-error Bun runtime API
const projectRoot = `${import.meta.dir}/..`;
const packageJsonPath = `${projectRoot}/package.json`;

async function getAvailableLifecycleScripts() {
	const pkg = JSON.parse(await Bun.file(packageJsonPath).text());
	const lifecycleScripts = [
		"preinstall",
		"install",
		"postinstall",
		"preprepare",
		"prepare",
		"postprepare",
	];

	const available = lifecycleScripts.filter((script) => pkg.scripts?.[script]);

	return {
		available,
		scripts: pkg.scripts || {},
	};
}

async function runScript(scriptName: string): Promise<void> {
	const pkg = JSON.parse(await Bun.file(packageJsonPath).text());
	const script = pkg.scripts?.[scriptName];

	if (!script) {
		console.log(`✗ Script "${scriptName}" not found in package.json`);
		return;
	}

	console.log(`\n► Running: ${scriptName}`);
	console.log(`  ${script}\n`);

	const proc = Bun.spawn(["sh", "-c", script], {
		cwd: projectRoot,
		stdio: ["inherit", "inherit", "inherit"],
	});

	const exitCode = await proc.exited;

	if (exitCode === 0) {
		console.log(`✓ ${scriptName} completed\n`);
	} else {
		throw new Error(`${scriptName} failed with code ${exitCode}`);
	}
}

const command = process.argv[2];

if (!command || command === "list") {
	const { available, scripts } = await getAvailableLifecycleScripts();

	console.log("\n📦 Available Lifecycle Scripts:\n");

	if (available.length === 0) {
		console.log("  (none configured)\n");
	} else {
		available.forEach((script) => {
			console.log(`  • ${script}`);
			console.log(`    ${scripts[script]}\n`);
		});
	}

	process.exit(0);
}

if (command === "postinstall") {
	try {
		await runScript("postinstall");
		process.exit(0);
	} catch (err) {
		console.error(`✗ Error:`, (err as Error).message);
		process.exit(1);
	}
} else if (command === "all") {
	try {
		const scripts = [
			"preinstall",
			"install",
			"postinstall",
			"preprepare",
			"prepare",
			"postprepare",
		];

		for (const script of scripts) {
			const pkg = JSON.parse(await Bun.file(packageJsonPath).text());
			if (pkg.scripts?.[script]) {
				await runScript(script);
			}
		}

		process.exit(0);
	} catch (err) {
		console.error(`✗ Error:`, (err as Error).message);
		process.exit(1);
	}
} else {
	console.log("\nUsage:");
	console.log(
		"  bun run scripts/postinstall.ts          # List available scripts",
	);
	console.log(
		"  bun run scripts/postinstall.ts list     # List available scripts",
	);
	console.log(
		"  bun run scripts/postinstall.ts postinstall  # Run postinstall",
	);
	console.log(
		"  bun run scripts/postinstall.ts all      # Run all lifecycle scripts\n",
	);
	process.exit(1);
}
