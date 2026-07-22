/**
 * Run an Ignition deployment with its progress UI redirected to stderr.
 *
 * Hardhat Ignition's script UI writes directly to process.stdout. Contract
 * commands reserve stdout for their final machine-readable JSON result, so we
 * temporarily route those writes to stderr while preserving displayUi: true.
 */
let pendingUiDeployment: Promise<void> = Promise.resolve();

export async function withIgnitionUiOnStderr<T>(
  deploy: () => Promise<T>
): Promise<T> {
  const previousUiDeployment = pendingUiDeployment;
  let releaseUiDeployment!: () => void;
  pendingUiDeployment = new Promise<void>((resolve) => {
    releaseUiDeployment = resolve;
  });
  await previousUiDeployment;

  const originalWrite = process.stdout.write;
  process.stdout.write = process.stderr.write.bind(
    process.stderr
  ) as typeof process.stdout.write;
  try {
    return await deploy();
  } finally {
    process.stdout.write = originalWrite;
    releaseUiDeployment();
  }
}
