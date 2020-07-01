/**
* @Author: Dylan
* @Date: 2020/7/1 10:41
 */

package main

import (
	"fmt"
	"testing"

	concurrentMap "github.com/fanliao/go-concurrentMap"
)

func TestHandle_ServeHTTP(t *testing.T) {

	testMap := concurrentMap.NewConcurrentMap()

	testMap.Put(11111, bodyContent{"1", 0})

	a, e := testMap.Get(11111)

	fmt.Println(a.(bodyContent).body, e)

}
