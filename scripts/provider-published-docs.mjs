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
