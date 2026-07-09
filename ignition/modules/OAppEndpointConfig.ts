import { buildModule } from "@nomicfoundation/hardhat-ignition/modules";
import type { Artifact } from "@nomicfoundation/ignition-core";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const endpointArtifact = require(
  "../../node_modules/@layerzerolabs/lz-evm-protocol-v2/artifacts/contracts/interfaces/ILayerZeroEndpointV2.sol/ILayerZeroEndpointV2.json",
) as Artifact;
const oAppCoreArtifact = require(
  "../../node_modules/@layerzerolabs/lz-evm-oapp-v2/artifacts/contracts/oapp/interfaces/IOAppCore.sol/IOAppCore.json",
) as Artifact;
const oAppOptionsArtifact = require(
  "../../node_modules/@layerzerolabs/lz-evm-oapp-v2/artifacts/contracts/oapp/interfaces/IOAppOptionsType3.sol/IOAppOptionsType3.json",
) as Artifact;

const OAppEndpointConfigModule = buildModule("OAppEndpointConfig", (m) => {
  const defaultSender = m.getAccount(0);
  const oappAddress = m.getParameter<string>("oapp");
  const endpointAddress = m.getParameter<string>("endpoint");
  const delegate = m.getParameter("delegate", defaultSender);
  const remoteEid = m.getParameter<number>("remoteEid");
  const remotePeer = m.getParameter<string>("remotePeer");
  const sendUln = m.getParameter<string>("sendUln");
  const receiveUln = m.getParameter<string>("receiveUln");
  const receiveLibraryGracePeriod = m.getParameter(
    "receiveLibraryGracePeriod",
    0,
  );
  const sendConfig = m.getParameter("sendConfig");
  const receiveConfig = m.getParameter("receiveConfig");
  const enforcedOptions = m.getParameter("enforcedOptions");

  const oappCore = m.contractAt("IOAppCore", oAppCoreArtifact, oappAddress);
  const oappOptions = m.contractAt(
    "IOAppOptionsType3",
    oAppOptionsArtifact,
    oappAddress,
  );
  const endpoint = m.contractAt(
    "ILayerZeroEndpointV2",
    endpointArtifact,
    endpointAddress,
  );

  const setPeer = m.call(oappCore, "setPeer", [remoteEid, remotePeer], {
    id: "SetOAppPeer",
  });
  const setSendLibrary = m.call(
    endpoint,
    "setSendLibrary",
    [oappCore, remoteEid, sendUln],
    { id: "SetEndpointSendLibrary", after: [setPeer] },
  );
  const setReceiveLibrary = m.call(
    endpoint,
    "setReceiveLibrary",
    [oappCore, remoteEid, receiveUln, receiveLibraryGracePeriod],
    { id: "SetEndpointReceiveLibrary", after: [setSendLibrary] },
  );
  const setSendConfig = m.call(
    endpoint,
    "setConfig",
    [oappCore, sendUln, sendConfig],
    { id: "SetEndpointSendConfig", after: [setReceiveLibrary] },
  );
  const setReceiveConfig = m.call(
    endpoint,
    "setConfig",
    [oappCore, receiveUln, receiveConfig],
    { id: "SetEndpointReceiveConfig", after: [setSendConfig] },
  );
  const setEnforcedOptions = m.call(oappOptions, "setEnforcedOptions", [enforcedOptions], {
    id: "SetOAppEnforcedOptions",
    after: [setReceiveConfig],
  });
  m.call(oappCore, "setDelegate", [delegate], {
    id: "SetOAppDelegate",
    after: [setEnforcedOptions],
  });

  return { oappCore, oappOptions, endpoint };
});

export default OAppEndpointConfigModule;
