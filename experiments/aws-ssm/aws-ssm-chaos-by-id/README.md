## Experiment Metadata

<table>
<tr>
<th> Name </th>
<th> Description </th>
<th> Documentation Link </th>
</tr>
<tr>
 <td> AWS SSM Chaos By ID </td>
 <td> This experiment causes the chaos injection on AWS resources using Amazon SSM Run Command. This is carried out by using SSM Docs that defines the actions performed by Systems Manager on your managed instances (having SSM agent installed) which let us perform chaos experiments on resources. In this experiment a default SSM docs is used to perform resource stress chaos over the ec2 instances defined by target instace ID(s). One can also provide its own SSM docs mounted as configmap and with the path defined with `DOCUMENT_PATH` ENV. One or more target instance can be provided in the list format in `EC2_INSTANCE_ID` env as comma(,) seperated envs (eg: instance1,instance2)</td>
 <td> <a href="https://litmuschaos.github.io/litmus/experiments/categories/aws-ssm/aws-ssm-chaos-by-id/"> Here </a> </td>
</tr>
</table>
