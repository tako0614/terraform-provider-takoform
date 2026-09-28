import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { renderFromSource } from "./provider-container-forward-mapping.mjs";

const root = resolve(import.meta.dirname, "..");

test("the pinned Go Forms candidate remains an unpublished typed Provider input", () => {
	const input = readFileSync(join(root, "internal/provider/container-forward-input.json"));
	const source = readFileSync(join(root, "cmd/provider-container-candidate/container-http-candidate.json"));
	const closure = readFileSync(join(root, "cmd/provider-container-candidate/closure.json"));
	const rendered = renderFromSource(source, input);
	expect(rendered.sourceBytes.equals(source)).toBe(true);
	expect(rendered.closureBytes.equals(closure)).toBe(true);
});

test("the candidate refuses drift in the exact binding roster or Container identity", () => {
	const input = readFileSync(join(root, "internal/provider/container-forward-input.json"));
	const source = JSON.parse(readFileSync(join(root, "cmd/provider-container-candidate/container-http-candidate.json"), "utf8"));
	for (const mutate of [
		candidate => { candidate.workerVersion.definition.acceptedBindings.pop(); },
		candidate => { candidate.formRef.schemaDigest = `sha256:${"0".repeat(64)}`; },
		candidate => {
			const definition = JSON.parse(candidate.workerDeployment.definitionJson);
			definition.desiredSchema.properties.versions.items.properties.workerVersion["x-takoform-target-formrefs"][0].schemaDigest = `sha256:${"0".repeat(64)}`;
			candidate.workerDeployment.definitionJson = JSON.stringify(definition);
		},
		candidate => {
			const definition = JSON.parse(candidate.vector.form.definitionJson);
			definition.immutableFields.pop();
			candidate.vector.form.definitionJson = JSON.stringify(definition);
		},
	]) {
		const candidate = structuredClone(source);
		mutate(candidate);
		expect(() => renderFromSource(Buffer.from(JSON.stringify(candidate)), input)).toThrow();
	}
});
