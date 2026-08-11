package modules

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	wasiBrokerModuleName       = "strata_broker"
	wasiBrokerCallName         = "call"
	maxWASIBrokerOperationSize = uint32(128)
	maxWASIBrokerPayloadSize   = uint32(1 << 20)
	wasiBrokerLengthPrefixSize = uint32(4)
)

const (
	wasiBrokerStatusOK uint32 = iota
	wasiBrokerStatusInvalid
	wasiBrokerStatusDenied
	wasiBrokerStatusUnknownOperation
	wasiBrokerStatusTooLarge
	wasiBrokerStatusBackendFailure
	wasiBrokerStatusUnavailable
)

func (r *WASIRuntime) instantiateBrokerHost(ctx context.Context, engine wazero.Runtime, module InstalledModule, scope ResourceScope, allowed bool) error {
	if r == nil || r.broker == nil {
		return ErrRuntimeBrokerUnavailable
	}
	builder := engine.NewHostModuleBuilder(wasiBrokerModuleName)
	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, guest api.Module, operationPtr, operationLen, inputPtr, inputLen, outputPtr, outputCap uint32) uint32 {
		return r.callBrokerHost(ctx, guest, module, scope, allowed, operationPtr, operationLen, inputPtr, inputLen, outputPtr, outputCap)
	}).Export(wasiBrokerCallName)
	if _, err := builder.Instantiate(ctx); err != nil {
		return fmt.Errorf("instantiate module broker host: %w", err)
	}
	return nil
}

func (r *WASIRuntime) callBrokerHost(
	ctx context.Context,
	guest api.Module,
	module InstalledModule,
	scope ResourceScope,
	allowed bool,
	operationPtr, operationLen, inputPtr, inputLen, outputPtr, outputCap uint32,
) uint32 {
	if !allowed || r == nil || r.broker == nil {
		return wasiBrokerStatusUnavailable
	}
	if guest == nil || guest.Memory() == nil {
		return wasiBrokerStatusInvalid
	}
	if operationLen == 0 || operationLen > maxWASIBrokerOperationSize {
		return wasiBrokerStatusInvalid
	}
	if inputLen > maxWASIBrokerPayloadSize || outputCap < wasiBrokerLengthPrefixSize || outputCap > maxWASIBrokerPayloadSize+wasiBrokerLengthPrefixSize {
		return wasiBrokerStatusTooLarge
	}

	memory := guest.Memory()
	operationBytes, ok := memory.Read(operationPtr, operationLen)
	if !ok {
		return wasiBrokerStatusInvalid
	}
	if bytes.IndexByte(operationBytes, 0) >= 0 {
		return wasiBrokerStatusInvalid
	}
	operation := string(append([]byte(nil), operationBytes...))
	if strings.TrimSpace(operation) != operation {
		return wasiBrokerStatusInvalid
	}
	input, ok := memory.Read(inputPtr, inputLen)
	if !ok {
		return wasiBrokerStatusInvalid
	}
	outputBuffer, ok := memory.Read(outputPtr, outputCap)
	if !ok {
		return wasiBrokerStatusInvalid
	}

	output, err := r.broker.Call(ctx, module, BrokerRequest{
		Operation: operation,
		Scope:     scope,
		Input:     append([]byte(nil), input...),
	})
	if err != nil {
		return brokerABIStatusForError(err)
	}
	if len(output)+4 > len(outputBuffer) {
		return wasiBrokerStatusTooLarge
	}

	var outputLen uint32
	for range output {
		outputLen++
	}
	binary.LittleEndian.PutUint32(outputBuffer[:4], outputLen)
	copy(outputBuffer[4:], output)
	return wasiBrokerStatusOK
}

func brokerABIStatusForError(err error) uint32 {
	switch {
	case errors.Is(err, ErrBrokerOperationUnknown):
		return wasiBrokerStatusUnknownOperation
	case errors.Is(err, ErrPermissionDenied),
		errors.Is(err, ErrModuleDisabled),
		errors.Is(err, ErrQuarantined),
		errors.Is(err, ErrBrokerVersionMismatch),
		errors.Is(err, ErrBrokerScopeInvalid):
		return wasiBrokerStatusDenied
	case errors.Is(err, ErrBrokerInputTooLarge), errors.Is(err, ErrBrokerOutputTooLarge):
		return wasiBrokerStatusTooLarge
	default:
		return wasiBrokerStatusBackendFailure
	}
}
