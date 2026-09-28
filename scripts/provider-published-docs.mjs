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
  ];
  for (const { issue, status } of patterns) {
    const versionThenStatus = new RegExp(
      `\\b${publishedVersion}\\b[^.\\n]{0,140}\\b${status}\\b`,
      "i",
    );
    const statusThenVersion = new RegExp(
      `\\b${status}\\s+(?:Provider\\s+)?\\b${publishedVersion}\\b`,
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
