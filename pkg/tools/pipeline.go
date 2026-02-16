package tools

import (
	"context"
	"fmt"
)

type ProcessingPipeline struct {
	stages []pipelineStage
}

type pipelineStage struct {
	name      string
	processor Processor
}

func NewPipeline() *ProcessingPipeline {
	return &ProcessingPipeline{
		stages: make([]pipelineStage, 0),
	}
}

func (p *ProcessingPipeline) AddStage(name string, processor Processor) Pipeline {
	p.stages = append(p.stages, pipelineStage{
		name:      name,
		processor: processor,
	})
	return p
}

func (p *ProcessingPipeline) Execute(ctx context.Context, input any) (any, error) {
	var err error
	output := input

	for _, stage := range p.stages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		output, err = stage.processor(ctx, output)
		if err != nil {
			return nil, fmt.Errorf("stage %s: %w", stage.name, err)
		}
	}

	return output, nil
}

func ContentTypeStage() Processor {
	return func(ctx context.Context, input any) (any, error) {
		switch v := input.(type) {
		case *FetchedContent:
			v.ContentType = DetectContentType(v.Data, v.MIMEType)
			return v, nil
		default:
			return input, nil
		}
	}
}

type FetchedContent struct {
	URL         string
	MIMEType    string
	ContentType ContentType
	Data        []byte
	Headers     map[string]string
}
