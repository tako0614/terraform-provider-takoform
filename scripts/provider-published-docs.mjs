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
  return null;
}
