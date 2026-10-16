package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

 var (
	grpcHandled = NewCounter(
	  	"grpc_request_total",
		"Total request completed",
		"method", "code")
	grpcDuration = NewHistogram(
		"rpc_server_duration_secconds",
		"gRPC request duration by method",
		"method")
	
 )

 func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
 
		grpcHandled.With(prometheus.Labels{
			"method": info.FullMethod,
			"code":   status.Code(err).String(),
		}).Inc()
 
		grpcDuration.With(prometheus.Labels{
			"method": info.FullMethod,
		}).Observe(time.Since(start).Seconds())
 
		return resp, err
	}
 }