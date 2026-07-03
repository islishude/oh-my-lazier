import { isAddress, type Address } from "viem";

export type LocalChainDeployment = {
  key: "a" | "b";
  name: string;
  eid: number;
  chainId: number;
  hostRpcUrl: string;
  containerRpcUrl: string;
  endpoint: Address;
  sendUln: Address;
  receiveUln: Address;
  oft: Address;
  openExecutor: Address;
  primaryOpenDVN: Address;
  secondaryOpenDVN: Address;
};

export type LocalE2EDeployment = {
  generatedAt: string;
  deployer: Address;
  worker: Address;
  parameters: {
    confirmations: string;
    maxMessageSize: number;
    minLzReceiveGas: string;
    lzReceiveGas: string;
    maxLzReceiveGas: string;
  };
  chains: {
    a: LocalChainDeployment;
    b: LocalChainDeployment;
  };
};

export function validateLocalE2EDeployment(
  value: unknown,
): LocalE2EDeployment {
  const deployment = object(value, "deployment");
  stringField(deployment, "generatedAt", "deployment.generatedAt");
  addressField(deployment, "deployer", "deployment.deployer");
  addressField(deployment, "worker", "deployment.worker");
  const parameters = object(
    deployment.parameters,
    "deployment.parameters",
  ) as LocalE2EDeployment["parameters"];
  unsignedDecimal(parameters.confirmations, "deployment.parameters.confirmations");
  numberField(parameters, "maxMessageSize", "deployment.parameters.maxMessageSize");
  unsignedDecimal(parameters.minLzReceiveGas, "deployment.parameters.minLzReceiveGas");
  unsignedDecimal(parameters.lzReceiveGas, "deployment.parameters.lzReceiveGas");
  unsignedDecimal(parameters.maxLzReceiveGas, "deployment.parameters.maxLzReceiveGas");
  const chains = object(deployment.chains, "deployment.chains");
  validateChain(chains.a, "deployment.chains.a", "a");
  validateChain(chains.b, "deployment.chains.b", "b");
  return deployment as LocalE2EDeployment;
}

function validateChain(value: unknown, path: string, key: "a" | "b") {
  const chain = object(value, path);
  if (chain.key !== key) {
    throw new Error(`${path}.key must be ${key}`);
  }
  stringField(chain, "name", `${path}.name`);
  numberField(chain, "eid", `${path}.eid`);
  numberField(chain, "chainId", `${path}.chainId`);
  stringField(chain, "hostRpcUrl", `${path}.hostRpcUrl`);
  stringField(chain, "containerRpcUrl", `${path}.containerRpcUrl`);
  for (const field of [
    "endpoint",
    "sendUln",
    "receiveUln",
    "oft",
    "openExecutor",
    "primaryOpenDVN",
    "secondaryOpenDVN",
  ]) {
    addressField(chain, field, `${path}.${field}`);
  }
}

function object(value: unknown, path: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${path} must be an object`);
  }
  return value as Record<string, unknown>;
}

function stringField(
  objectValue: Record<string, unknown>,
  field: string,
  path: string,
) {
  if (typeof objectValue[field] !== "string" || objectValue[field] === "") {
    throw new Error(`${path} must be a non-empty string`);
  }
}

function numberField(
  objectValue: Record<string, unknown>,
  field: string,
  path: string,
) {
  if (
    typeof objectValue[field] !== "number" ||
    !Number.isInteger(objectValue[field]) ||
    objectValue[field] <= 0
  ) {
    throw new Error(`${path} must be a positive integer`);
  }
}

function addressField(
  objectValue: Record<string, unknown>,
  field: string,
  path: string,
) {
  if (typeof objectValue[field] !== "string" || !isAddress(objectValue[field])) {
    throw new Error(`${path} must be an EVM address`);
  }
}

function unsignedDecimal(value: unknown, path: string) {
  if (typeof value !== "string" || !/^[0-9]+$/.test(value)) {
    throw new Error(`${path} must be an unsigned decimal string`);
  }
}
