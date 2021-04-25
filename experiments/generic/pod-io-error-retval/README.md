## Experiment Metadata

A more detailed design document can be found [here](https://github.com/arindas/litmus-go/blob/experiment/io-error/chaoslib/litmus/pod-io-error-retval/README.md)

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> Pod IO Error Retval </td>
 <td> This experiment causes IO errors by using the Linux Kernels's debugfs fail_function feature. We inject failures into application code by instructing the kernel to return errors like ENOMEM, EIO etc. when specific syscalls like read(), open() etc. are called.  </td>
 <td>  <a href=""> Coming soon </a> </td>
 </tr>
 </table>
