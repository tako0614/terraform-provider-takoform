export function stalePublishedProviderStatus(source, version) {
  const escapedVersion = version.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const publishedVersion = `v?${escapedVersion}`;
  const patterns = [
    {
      issue: "stale unpublished provider status",
      status: "(?:unpublished|unavailable)",
    },
    {
      issue: "stale not-installable provider status",
      status: "not (?:yet )?installable",
    },
    {
      issue: "stale candidate provider status",
      status: "(?:still (?:a )?)?candidate|not (?:yet )?published",
    },
  ];
  for (const { issue, status } of patterns) {
    const versionThenStatus = new RegExp(
      `\\b${publishedVersion}\\b[^.\\n]{0,140}\\b(?:${status})\\b`,
      "i",
    );
    const statusThenVersion = new RegExp(
      `\\b(?:${status})\\s+(?:Provider\\s+)?\\b${publishedVersion}\\b`,
      "i",
    );
    if (versionThenStatus.test(source) || statusThenVersion.test(source)) {
      return issue;
    }
  }
  const japaneseVersionThenStatus = new RegExp(
    `\\b${publishedVersion}\\b[^.。\\n]{0,140}(?:未公開|未提供|利用不可)`,
  );
  const japaneseStatusThenVersion = new RegExp(
    `(?:未公開|未提供|利用不可)(?:の|な)?\\s*(?:Provider\\s*)?\\b${publishedVersion}\\b`,
  );
  if (japaneseVersionThenStatus.test(source) || japaneseStatusThenVersion.test(source)) {
    return "stale unpublished provider status";
  }
  return null;
}

export const PUBLISHED_INSTALL_PROSE = [
  "README.md",
  "docs/index.md",
  "website/index.md",
  "website/ja/index.md",
  "website/docs/index.md",
  "website/ja/docs/index.md",
];

export function publishedProviderInstallDocStatusIssues(sources, version) {
  return PUBLISHED_INSTALL_PROSE.flatMap((relativePath) => {
    const issue = stalePublishedProviderStatus(sources.get(relativePath) ?? "", version);
    return issue === null ? [] : [`${relativePath}: ${issue}`];
  });
}

export function releaseTargetTagDocIssues(source, targetVersion) {
  const issues = [];
  const targetStatus = stalePublishedProviderStatus(source, targetVersion);
  if (targetStatus !== null) {
    issues.push(`release target ${targetVersion} has ${targetStatus}`);
  }
  const checkout = source.match(/^git checkout --detach v([^\s]+)$/m)?.[1] ?? null;
  if (checkout !== targetVersion) {
    issues.push(checkout === null
      ? `release target ${targetVersion} docs omit the exact v${targetVersion} checkout`
      : `release target ${targetVersion} docs check out v${checkout} instead of v${targetVersion}`);
  }
  return issues;
}

// The source docs captured by a Provider 5 tag must describe the selected
// breaking target. Published Provider 4.1 remains the installation readback;
// it is not a substitute for the new tag's own adoption reference.
function h2Sections(source, heading) {
  const headings = [...source.matchAll(/^## ([^\r\n]+)\r?$/gm)];
  return headings.flatMap((match, index) => heading.test(match[1])
    ? [source.slice(match.index, headings[index + 1]?.index ?? source.length)]
    : []);
}

function exactCheckoutIssues(section, version, label) {
  const checkouts = [...section.matchAll(/^git checkout --detach v([^\s]+)$/gm)]
    .map((match) => match[1]);
  if (checkouts.length === 0) {
    return [`${label} docs omit the exact v${version} checkout`];
  }
  if (checkouts.length !== 1) {
    return [`${label} docs contain multiple tag checkouts`];
  }
  return checkouts[0] === version ? []
    : [`${label} docs check out v${checkouts[0]} instead of v${version}`];
}

export function publishedProviderTagDocIssues(source, publishedVersion) {
  const sections = h2Sections(source, /^Verify the release$/u);
  if (sections.length !== 1) {
    return [`published Provider ${publishedVersion} docs ${sections.length === 0 ? "omit" : "duplicate"} the verification section`];
  }
  return exactCheckoutIssues(sections[0], publishedVersion, `published Provider ${publishedVersion}`);
}

export function provider5TagDocIssues(source, descriptor, candidate) {
  const issues = [];
  const escapedVersion = descriptor.version.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const sections = h2Sections(source, new RegExp(`^Provider ${escapedVersion}(?: .*)?$`, "u"));
  if (sections.length !== 1) {
    return [`release target ${descriptor.version} docs ${sections.length === 0 ? "omit a dedicated" : "duplicate the"} Provider ${descriptor.version} section`];
  }
  const targetSource = sections[0];
  issues.push(...exactCheckoutIssues(targetSource, descriptor.version, `release target ${descriptor.version}`));
  const major = descriptor.version.split(".")[0];
  const selectedPin = new RegExp(`^\\s*version\\s*=\\s*"(?:~>\\s*${major}\\.0|=\\s*${escapedVersion})"`, "m");
  if (!selectedPin.test(targetSource)) {
    issues.push(`release target ${descriptor.version} docs omit a v${major} HCL provider pin`);
  }
  if (!targetSource.includes(descriptor.versioning.portableApiVersion)) {
    issues.push(`release target ${descriptor.version} docs omit Host API ${descriptor.versioning.portableApiVersion}`);
  }
  if (!candidate.forms.some(({ formURL }) => targetSource.includes(formURL))) {
    issues.push(`release target ${descriptor.version} docs omit an exact publisher Form URL`);
  }
  if (!candidate.forms.some(({ resourceType }) => targetSource.includes(`resource "${resourceType}"`))) {
    issues.push(`release target ${descriptor.version} docs omit a registered v${major} HCL resource`);
  }
  if (!targetSource.includes("Provider 4 state is not automatically translated to Provider 5.")) {
    issues.push(`release target ${descriptor.version} docs omit the Provider 4 state boundary`);
  }
  return issues;
}
