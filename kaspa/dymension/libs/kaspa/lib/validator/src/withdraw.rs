// We call the signers 'validators'

use corelib::escrow::*;
use std::collections::hash_map::Entry;

use kaspa_core;
use kaspa_wallet_core::error::Error;

use kaspa_wallet_pskt::prelude::*;
use secp256k1::Keypair as SecpKeypair;

use crate::error::ValidationError;
use corelib::payload::{MessageID, MessageIDs};
use corelib::util;
use corelib::util::{get_recipient_address, get_recipient_script_pubkey, is_valid_sighash_type};
use corelib::wallet::EasyKaspaWallet;
use corelib::withdraw::{filter_pending_withdrawals, WithdrawFXG};
use eyre::{Report, Result};
use hex::ToHex;
use hyperlane_core::HyperlaneDomainConfigError::DomainNameMismatch;
use hyperlane_core::{Decode, HyperlaneDomain, HyperlaneMessage, KnownHyperlaneDomain, H256, U256};
use hyperlane_cosmos_native::GrpcProvider as CosmosGrpcClient;
use hyperlane_cosmos_rs::dymensionxyz::dymension::kas::{WithdrawalId, WithdrawalStatus};
use hyperlane_warp_route::TokenMessage;
use kaspa_addresses::{Address as KaspaAddress, Prefix as KaspaAddrPrefix};
use kaspa_consensus_core::hashing::sighash::{
    calc_schnorr_signature_hash, SigHashReusedValuesUnsync,
};
use kaspa_consensus_core::mass::transaction_output_estimated_serialized_size;
use kaspa_consensus_core::tx::{ScriptPublicKey, TransactionOutpoint, TransactionOutput};
use kaspa_hashes;
use kaspa_txscript::pay_to_address_script;
use kaspa_wallet_core::utxo::NetworkParams;
use std::collections::HashMap;
use std::io::Cursor;
use tracing::{debug, error, info, warn};

#[derive(Clone)]
pub struct MustMatch {
    address_prefix: KaspaAddrPrefix,
    escrow_public: EscrowPublic,
    partial_message: HyperlaneMessage,
    hub_mailbox_id: String,
}

impl MustMatch {
    pub fn new(
        address_prefix: KaspaAddrPrefix,
        escrow_public: EscrowPublic,
        hub_domain: u32,
        hub_token_id: H256,
        kas_domain: u32,
        kas_token_placeholder: H256, // a fake value, since Kaspa does not have a 'token' smart contract. Howevert his value must be consistent with hub config.
        hub_mailbox_id: String,
    ) -> Self {
        Self {
            address_prefix,
            escrow_public,
            partial_message: HyperlaneMessage {
                version: 0,
                nonce: 0,
                origin: hub_domain,
                sender: hub_token_id,
                destination: kas_domain,
                recipient: kas_token_placeholder,
                body: vec![],
            },
            hub_mailbox_id,
        }
    }

    fn is_match(&self, other: &HyperlaneMessage) -> bool {
        self.partial_message.origin == other.origin
            && self.partial_message.sender == other.sender
            && self.partial_message.destination == other.destination
            && self.partial_message.recipient == other.recipient
    }
}

/// Validate WithdrawFXG received from the relayer against Kaspa and Hub.
/// It verifies that:
/// (0)  All messages should have Kaspa domain.
/// (1)  No double spending allowed. All messages must be unique.
/// (2)  Each message is actually dispatched on the Hub. Achieved by `CosmosGrpcClient.delivered`.
///      Consequence: `delivered` ensures that the HL message hash in known on the Hub,
///      which verifies the correctness of all the HL message fields.
/// (3)  The messages are not yet marked as processed on the Hub.
/// (4)  The anchor UTXO provided by the relayer is actually still the anchor on the Hub.
/// (5)  The Kaspa TXs are a linked sequence. The first PSKT contains Hub anchor in inputs.
/// (6)  Check PSKT:
///      - The Kaspa TXs have corresponding message IDs in their payload (msg ID == msg hash).
///        Consequence: Each message actually hashes to the hash stored in the payload.
///      - Correct sighash type in inputs
///      - No lock time
///      - TX version
/// (7)  TX UTXO spends actually correspond to the message content.
/// (8)  No message use escrow as a recipient.
/// (9)  Each PSKT has exactly one anchor.
///
/// CONTRACT: the first anchor of `fxg.anchors` is the Hub anchor.
pub async fn validate_withdrawal_batch(
    fxg: &WithdrawFXG,
    cosmos_client: &CosmosGrpcClient,
    must_match: MustMatch,
) -> Result<(), ValidationError> {
    let hub_anchor = validate_messages(fxg, cosmos_client, &must_match).await?;

    // At this point we know
    // - The set of messages is unique
    // - All the messages are dispatched on the hub
    // - None of the messages are already confirmed on the hub

    validate_pskts(fxg, hub_anchor, must_match)
        .map_err(|e| eyre::eyre!("WithdrawFXG validation failed: {}", e))?;

    info!("Withdrawal validation completed successfully for withdrawals");

    Ok(())
}

async fn validate_messages(
    fxg: &WithdrawFXG,
    cosmos_client: &CosmosGrpcClient,
    must_match: &MustMatch,
) -> Result<TransactionOutpoint, ValidationError> {
    let messages: Vec<HyperlaneMessage> = fxg.messages.clone().into_iter().flatten().collect();
    let num_msgs = messages.len();
    debug!(
        "Starting withdrawal validation for messages, num_msgs: {}",
        num_msgs
    );
    let msg_ids: Vec<H256> = messages.iter().map(|m| m.id()).collect();
    if let Some(duplicate) = util::find_duplicate(&msg_ids) {
        let message_id = duplicate.encode_hex();
        return Err(ValidationError::DoubleSpending { message_id });
    }
    for msg in messages.iter() {
        if !must_match.is_match(&msg) {
            return Err(ValidationError::MessageWrongBridge {
                message_id: msg.id().encode_hex(),
            });
        }
    }
    for id in msg_ids {
        let res = cosmos_client
            .delivered(must_match.hub_mailbox_id.clone(), id.encode_hex())
            .await
            .map_err(|e| ValidationError::SystemError(Report::from(e)))?;

        // Delivered is a confusing name. `delivered` is just the name of the network query.
        let was_dispatched_on_hub = res.delivered;
        info!("was_dispatched_on_hub: {}", was_dispatched_on_hub);
        if !was_dispatched_on_hub {
            let message_id = id.encode_hex();
            return Err(ValidationError::MessageNotDispatched { message_id });
        }
    }
    debug!("All withdrawal fxg messages are dispatched on hub");
    let (hub_anchor, pending_messages) = filter_pending_withdrawals(messages, cosmos_client, None)
        .await
        .map_err(|e| eyre::eyre!("Get pending withdrawals: {}", e))?;
    if num_msgs != pending_messages.len() {
        return Err(ValidationError::MessagesNotUnprocessed);
    }
    debug!("All withdrawal fxg messages are unprocessed on hub");
    Ok(hub_anchor)
}

pub fn validate_pskts(
    fxg: &WithdrawFXG,
    hub_anchor: TransactionOutpoint,
    must_match: MustMatch,
) -> Result<(), ValidationError> {
    if fxg.bundle.0.len() != fxg.messages.len() {
        return Err(ValidationError::MessageCacheLengthMismatch {
            expected: fxg.bundle.0.len(),
            actual: fxg.messages.len(),
        });
    }

    // PSKTs must be linked by anchor, starting with the current hub anchor
    let mut anchor_to_spend = hub_anchor;
    for (idx, pskt) in fxg.bundle.iter().enumerate() {
        let messages = fxg.messages.get(idx).unwrap();

        anchor_to_spend = validate_pskt(
            PSKT::<Signer>::from(pskt.clone()),
            anchor_to_spend,
            messages,
            must_match.clone(),
        )
        .map_err(|e| eyre::eyre!("Single PSKT validation failed: {}", e))?;
    }

    Ok(())
}

pub fn validate_pskt(
    pskt: PSKT<Signer>,
    must_spend: TransactionOutpoint,
    expected_messages: &Vec<HyperlaneMessage>,
    must_match: MustMatch,
) -> Result<TransactionOutpoint, ValidationError> {
    validate_pskt_impl_details(&pskt, &must_spend, expected_messages, &must_match)?;
    let ix = validate_pskt_application_semantics(&pskt, must_spend, expected_messages, must_match)?;
    Ok(TransactionOutpoint::new(pskt.calculate_id(), ix))
}

pub fn validate_pskt_impl_details(
    pskt: &PSKT<Signer>,
    must_spend: &TransactionOutpoint,
    expected_messages: &Vec<HyperlaneMessage>,
    must_match: &MustMatch,
) -> Result<(), ValidationError> {
    if pskt
        .inputs
        .iter()
        .any(|input| !is_valid_sighash_type(input.sighash_type))
    {
        return Err(ValidationError::SigHashType);
    }

    if pskt.global.fallback_lock_time.is_some() {
        return Err(ValidationError::LockTime);
    }

    Ok(())
}

pub fn validate_pskt_application_semantics(
    pskt: &PSKT<Signer>,
    must_spend: TransactionOutpoint,
    expected_messages: &Vec<HyperlaneMessage>,
    must_match: MustMatch,
) -> Result<u32, ValidationError> {
    if expected_messages.len() == 0 {
        return Err(ValidationError::NoMessages);
    }

    if !pskt
        .inputs
        .iter()
        .any(|input| input.previous_outpoint == must_spend)
    {
        return Err(ValidationError::AnchorNotFound { o: must_spend });
    }

    // Payload covers corresponding HL messages
    let payload = MessageIDs(
        expected_messages
            .iter()
            .map(|m| MessageID(m.id()))
            .collect(),
    )
    .to_bytes()
    .map_err(|e| eyre::eyre!("Failed to serialize MessageIDs: {}", e))?;

    let pskt_payload = pskt.global.payload.clone().unwrap_or(vec![]);

    if pskt_payload != payload {
        return Err(ValidationError::PayloadMismatch);
    }

    if pskt.global.tx_version != kaspa_consensus_core::constants::TX_VERSION {
        return Err(ValidationError::TxVersionMismatch);
    }

    // Check that UTXO outputs align with withdrawals
    // Find escrow input amount
    let escrow_input_amount = pskt.inputs.iter().fold(0, |acc, i| {
        // redeem_script is None for relayer input
        let rs = i.redeem_script.clone().unwrap_or_default();
        return if rs == must_match.escrow_public.redeem_script {
            acc + i.utxo_entry.as_ref().unwrap().amount
        } else {
            acc
        };
    });

    // Construct a multiset of expected outputs from HL messages.
    // Key:   recipiend + amount
    // Value: number of entries
    //
    // Such structure accounts for cases where one address might send several transfers
    // with the same amount.
    let mut expected_outputs: HashMap<(u64, ScriptPublicKey), i32> = HashMap::new();

    for m in expected_messages {
        let tm = TokenMessage::read_from(&mut Cursor::new(&m.body))
            .map_err(|e| eyre::eyre!("Failed to parse TokenMessage from message body: {}", e))?;

        let recipient = get_recipient_script_pubkey(tm.recipient(), must_match.address_prefix);

        // Step 8: Check that there are no withdrawals where escrow is set
        // as recepient. It would drastically complicate the confirmation flow.
        if recipient == must_match.escrow_public.p2sh {
            let message_id = m.id().encode_hex();
            return Err(ValidationError::EscrowWithdrawalNotAllowed { message_id });
        }

        let key = (tm.amount().as_u64(), recipient);
        *expected_outputs.entry(key).or_default() += 1;
    }

    // Ensure that all HL messages have outputs.
    // Also, calculate the total output amount of withdrawals + escrow change,
    // it should match the input escrow amount.
    let mut escrow_output_amount = 0;
    let mut next_anchor_idx: Option<u32> = None;
    for (idx, output) in pskt.outputs.iter().enumerate() {
        let key = (output.amount, output.script_public_key.clone());

        let e = expected_outputs.entry(key).and_modify(|v| *v -= 1);
        if let Entry::Occupied(e) = e {
            escrow_output_amount += output.amount;
            if *e.get() == 0 {
                e.remove();
            }
            continue;
        }

        // Check that output is an anchor
        if output.script_public_key == must_match.escrow_public.p2sh {
            // Step 9: Abort if there is more than one anchor candidate
            if next_anchor_idx.is_some() {
                return Err(ValidationError::MultipleAnchors);
            }

            escrow_output_amount += output.amount;
            next_anchor_idx = Some(idx as u32);
        }
    }

    // expected_outputs contains the number of occurrences of (recipiend; amount) pairs.
    // If it is empty, then all the occurrences are covered by the Kaspa TX.
    if !expected_outputs.is_empty() {
        return Err(ValidationError::MissingOutputs);
    }

    // Verify that the input of escrow funds equals to the output of escrow funds:
    // Input == output == escrow change + sum(withdrawals)
    if escrow_input_amount != escrow_output_amount {
        return Err(ValidationError::EscrowAmountMismatch {
            input_amount: escrow_input_amount,
            output_amount: escrow_output_amount,
        });
    }

    Ok(next_anchor_idx.ok_or(ValidationError::NextAnchorNotFound)?)
}

pub fn sign_withdrawal_fxg(fxg: &WithdrawFXG, keypair: &SecpKeypair) -> Result<Bundle> {
    let mut signed = Vec::new();
    for (pskt) in fxg.bundle.iter() {
        let pskt = PSKT::<Signer>::from(pskt.clone());

        let signed_pskt = corelib::pskt::sign_pskt(pskt, keypair, None)?;

        signed.push(signed_pskt);
    }
    info!("Validator: signed pskts");
    let bundle = Bundle::from(signed);
    Ok(bundle)
}
